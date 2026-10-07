package yunwu

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
)

var (
	ErrNoYunwuNodesAvailable = errors.New("no Yunwu execution nodes available")
	ErrNoCompatibleNodes     = errors.New("no execution nodes configured for model family")
	ErrNodeBalanceExhausted  = errors.New("selected execution node balance exhausted")
)

const (
	AffinityBonus           = 15.0
	PriorityBonusMultiplier = 5.0
)

type YunwuScheduler struct {
	requireFreshBalance bool
	// requiredHealthyStreak is the number of consecutive successful checks
	// (health probes or served production requests) a node must accumulate
	// after a failure before it is allowed to take traffic again. This
	// implements §6.1: a recovered Primary must not come back after a single
	// ping. 0 disables the gate (legacy behaviour).
	requiredHealthyStreak int
}

func NewYunwuScheduler(requireFreshBalance ...bool) *YunwuScheduler {
	requireFresh := false
	if len(requireFreshBalance) > 0 {
		requireFresh = requireFreshBalance[0]
	}
	return &YunwuScheduler{requireFreshBalance: requireFresh}
}

// SetRequiredHealthyStreak configures the recovery streak gate (see field doc).
func (s *YunwuScheduler) SetRequiredHealthyStreak(n int) *YunwuScheduler {
	if n < 0 {
		n = 0
	}
	s.requiredHealthyStreak = n
	return s
}

// isRecovering reports whether a node has a failure history and has not yet
// proven the required streak of consecutive successes since. LastErrorAt is the
// marker (not CooldownUntil, which the health controller clears once a cooldown
// expires — clearing it would let a node resume after a single ping).
func (s *YunwuScheduler) isRecovering(state *health.NodeRuntimeState) bool {
	if state == nil || state.LastErrorAt == nil || s.requiredHealthyStreak <= 0 {
		return false
	}
	return state.ConsecutiveSuccesses < s.requiredHealthyStreak
}

// SelectNode selects the best Yunwu execution node for a session based on Yunwu credit balance and health stats
func (s *YunwuScheduler) SelectNode(
	ctx context.Context,
	nodes []health.ExecutionNode,
	states map[string]*health.NodeRuntimeState,
	credits map[string]float64,
	userPriority int,
	currentLeaseNodeID string,
	systemState string,
	prices ...map[string]float64,
) (*health.ExecutionNode, error) {
	now := time.Now()
	var candidates []health.ExecutionNode
	// Nodes returning from a failure that have not yet proven a healthy streak.
	// They are held back (§6.1) but kept as a last-resort fallback so a pool of
	// only-recovering nodes still serves instead of hard-failing.
	var deferred []health.ExecutionNode

	// 1. Filter out unavailable, zero-credit (<0.01), and nodes in active cooldown
	for _, node := range nodes {
		if node.State == "unavailable" {
			slog.Debug("[YunwuScheduler] Excluding unavailable node", "node_id", node.NodeID)
			continue
		}

		// Check credit balance (if tracked)
		credit, tracked := credits[node.NodeID]
		if s.requireFreshBalance && (!tracked || credit <= 0) {
			slog.Warn("[YunwuScheduler] Excluding exhausted credit Yunwu node", "node_id", node.NodeID, "available_credit", credit)
			continue
		}
		if !s.requireFreshBalance && (!tracked || credit <= 0) {
			slog.Info("[YunwuScheduler] Scheduling Yunwu node without a usable balance snapshot", "node_id", node.NodeID)
		}

		// Check if in cooldown
		isCooldown := node.State == "cooldown"
		if state, exists := states[node.NodeID]; exists && state.CooldownUntil != nil {
			if now.Before(*state.CooldownUntil) {
				isCooldown = true
			}
		}

		if isCooldown && len(nodes) > 1 {
			slog.Debug("[YunwuScheduler] Excluding cooldown node", "node_id", node.NodeID)
			continue
		}

		// §6.1: a node with a failure history must prove a streak of consecutive
		// successful checks before it takes traffic again (never after a single ping).
		if s.isRecovering(states[node.NodeID]) {
			st := states[node.NodeID]
			slog.Info("[YunwuScheduler] Deferring recovering node until it proves a healthy streak",
				"node_id", node.NodeID,
				"consecutive_successes", st.ConsecutiveSuccesses,
				"required", s.requiredHealthyStreak,
			)
			deferred = append(deferred, node)
			continue
		}

		candidates = append(candidates, node)
	}

	if len(candidates) == 0 && s.requireFreshBalance {
		slog.Warn("[YunwuScheduler] All candidate Yunwu nodes were excluded due to fresh balance requirements; falling back to non-unavailable nodes", "total_nodes", len(nodes))
		for _, node := range nodes {
			if node.State == "unavailable" {
				continue
			}
			isCooldown := node.State == "cooldown"
			if state, exists := states[node.NodeID]; exists && state.CooldownUntil != nil {
				if now.Before(*state.CooldownUntil) {
					isCooldown = true
				}
			}
			if isCooldown && len(nodes) > 1 {
				continue
			}
			if s.isRecovering(states[node.NodeID]) {
				deferred = append(deferred, node)
				continue
			}
			candidates = append(candidates, node)
		}
	}

	// Last-resort safety valve: if every candidate is still recovering, serve from
	// it rather than failing the request outright.
	if len(candidates) == 0 && len(deferred) > 0 {
		slog.Warn("[YunwuScheduler] Only recovering nodes available; serving from recovering nodes as last resort",
			"recovering_count", len(deferred),
		)
		candidates = deferred
	}

	if len(candidates) == 0 {
		slog.Error("[YunwuScheduler] No candidate Yunwu nodes available", "total_nodes", len(nodes))
		return nil, ErrNoYunwuNodesAvailable
	}

	// 2. Separate healthy and limited candidates
	var healthyCandidates []health.ExecutionNode
	var limitedCandidates []health.ExecutionNode

	for _, node := range candidates {
		if node.State == "healthy" || node.State == "busy" || node.State == "" {
			healthyCandidates = append(healthyCandidates, node)
		} else if node.State == "limited" {
			limitedCandidates = append(limitedCandidates, node)
		}
	}

	var finalCandidates []health.ExecutionNode
	if len(healthyCandidates) > 0 {
		finalCandidates = healthyCandidates
	} else if len(limitedCandidates) > 0 {
		finalCandidates = limitedCandidates
	} else {
		finalCandidates = candidates
	}

	// 2.5 Filter by cheapest price tier (Cheapest-First Routing)
	// The node with the lowest price rate (inputPrice + outputPrice) is prioritized.
	// More expensive nodes act strictly as fallback candidates (used when the cheaper nodes are in cooldown or unavailable).
	if len(prices) > 0 && prices[0] != nil && len(prices[0]) > 0 {
		priceMap := prices[0]
		minPrice := -1.0
		for _, node := range finalCandidates {
			if p, ok := priceMap[node.NodeID]; ok {
				if minPrice < 0 || p < minPrice {
					minPrice = p
				}
			}
		}

		if minPrice >= 0 {
			var cheapestCandidates []health.ExecutionNode
			for _, node := range finalCandidates {
				if p, ok := priceMap[node.NodeID]; ok {
					if p <= minPrice*1.0001 || (p-minPrice) < 1e-9 {
						cheapestCandidates = append(cheapestCandidates, node)
					}
				}
			}
			if len(cheapestCandidates) > 0 {
				slog.Debug("[YunwuScheduler] Filtered candidates to cheapest price tier",
					"cheapest_count", len(cheapestCandidates),
					"total_candidates", len(finalCandidates),
					"min_price", minPrice,
				)
				finalCandidates = cheapestCandidates
			}
		}
	}

	// 3. Score Yunwu nodes:
	// score = success_rate * 100 + affinity + priority + credit_bonus - queue_depth * 5 - avg_latency / 100 - recent_429 * 30
	var bestNode *health.ExecutionNode
	var bestScore float64

	for i, node := range finalCandidates {
		state, exists := states[node.NodeID]
		successRate := 1.0
		var avgLatencyMs int64
		var queueDepth int
		var recent429 int
		var recentTimeout int

		if exists {
			successRate = state.SuccessRate
			avgLatencyMs = state.AvgLatencyMs
			queueDepth = state.QueueDepth
			recent429 = state.Recent429
			recentTimeout = state.RecentTimeouts
		}

		score := successRate * 100.0

		// + Affinity
		if currentLeaseNodeID != "" && node.NodeID == currentLeaseNodeID {
			score += AffinityBonus
		}

		// + User priority
		score += float64(userPriority) * PriorityBonusMultiplier

		// + Available credit bonus (capped at 50 points)
		credit := credits[node.NodeID]
		creditBonus := credit * 5.0
		if creditBonus > 50.0 {
			creditBonus = 50.0
		}
		score += creditBonus

		// - Queue depth
		score -= float64(queueDepth) * 5.0

		// - Used tokens organic penalty for fair distribution (max 25 points)
		if exists && state.UsedTokens > 0 {
			tokenPenalty := float64(state.UsedTokens) / 100000.0
			if tokenPenalty > 25.0 {
				tokenPenalty = 25.0
			}
			score -= tokenPenalty
		}

		// - Latency
		latencyDivider := 100.0
		if systemState == "WARNING" {
			latencyDivider = 50.0
		}
		score -= float64(avgLatencyMs) / latencyDivider

		// - 429 and timeouts
		multiplier429 := 30.0
		if systemState == "WARNING" {
			multiplier429 = 60.0
		}
		score -= float64(recent429) * multiplier429
		score -= float64(recentTimeout) * 20.0

		slog.Debug("[YunwuScheduler] Evaluated node candidate",
			"node_id", node.NodeID,
			"account_name", node.AccountName,
			"score", score,
			"credit", credit,
			"credit_bonus", creditBonus,
			"success_rate", successRate,
			"avg_latency_ms", avgLatencyMs,
			"recent_429", recent429,
		)

		if i == 0 || score > bestScore {
			bestScore = score
			bestNode = &finalCandidates[i]
		}
	}

	slog.Info("[YunwuScheduler] Selected best Yunwu execution node",
		"node_id", bestNode.NodeID,
		"account_name", bestNode.AccountName,
		"score", bestScore,
		"total_candidates_evaluated", len(finalCandidates),
	)

	return bestNode, nil
}
