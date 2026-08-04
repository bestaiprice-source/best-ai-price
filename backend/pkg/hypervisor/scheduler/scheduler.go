package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/health"
)

var (
	ErrNoNodesAvailable = errors.New("no execution nodes available")
)

const (
	AffinityBonus           = 15.0
	PriorityBonusMultiplier = 5.0
)

type Scheduler struct{}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

// SelectNode selects the best execution node for a session based on the score formula
func (s *Scheduler) SelectNode(ctx context.Context, nodes []health.ExecutionNode, states map[string]health.NodeRuntimeState, userPriority int, currentLeaseNodeID string, systemState string) (*health.ExecutionNode, error) {
	now := time.Now()
	var candidates []health.ExecutionNode

	// 1. Filter out unavailable and nodes in active cooldown
	for _, node := range nodes {
		if node.State == "unavailable" {
			slog.Debug("[Scheduler] Excluding unavailable node", "node_id", node.NodeID)
			continue
		}

		// Check if in cooldown either via state or runtime state CooldownUntil
		isCooldown := node.State == "cooldown"
		if state, exists := states[node.NodeID]; exists && state.CooldownUntil != nil {
			if now.Before(*state.CooldownUntil) {
				isCooldown = true
			}
		}

		if isCooldown && len(nodes) > 1 {
			slog.Debug("[Scheduler] Excluding cooldown node", "node_id", node.NodeID)
			continue
		}

		candidates = append(candidates, node)
	}

	if len(candidates) == 0 {
		slog.Warn("[Scheduler] All nodes in cooldown or filtered out, relaxing cooldown restriction for single pool node")
		for _, node := range nodes {
			if node.State != "unavailable" {
				candidates = append(candidates, node)
			}
		}
	}

	if len(candidates) == 0 {
		slog.Error("[Scheduler] No candidate execution nodes available in pool", "total_nodes", len(nodes))
		return nil, ErrNoNodesAvailable
	}

	// 2. Separate healthy/busy and limited nodes
	// "limited использовать только при отсутствии healthy" (healthy includes busy)
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
		slog.Warn("[Scheduler] Only limited nodes available, selecting from limited pool", "count", len(limitedCandidates))
		finalCandidates = limitedCandidates
	} else {
		finalCandidates = candidates
	}

	// 3. Compute score and pick the best node
	var bestNode *health.ExecutionNode
	var bestScore float64

	for i, node := range finalCandidates {
		state, exists := states[node.NodeID]
		successRate := 1.0 // default to 100% success rate if no state
		var avgLatencyMs int64
		var queueDepth int
		var recent429 int
		var recentTimeout int
		var usedTokens int64

		if exists {
			successRate = state.SuccessRate
			avgLatencyMs = state.AvgLatencyMs
			queueDepth = state.QueueDepth
			recent429 = state.Recent429
			recentTimeout = state.RecentTimeouts
			usedTokens = state.UsedTokens
		}

		// score = success_rate * 100
		score := successRate * 100.0

		// + affinity_bonus
		hasAffinity := false
		if currentLeaseNodeID != "" && node.NodeID == currentLeaseNodeID {
			score += AffinityBonus
			hasAffinity = true
		}

		// + priority_bonus
		score += float64(userPriority) * PriorityBonusMultiplier

		// - queue_depth * 5
		score -= float64(queueDepth) * 5.0

		// - used_tokens penalty for fair distribution
		// 10k tokens = 1 point penalty. This organically distributes load across nodes
		// without overriding health signals like success rate and latency.
		score -= float64(usedTokens) / 10000.0

		// Penalize more heavily based on degradation state
		multiplier429 := 30.0
		multiplierTimeout := 20.0
		latencyDivider := 100.0

		if systemState == "WARNING" {
			multiplier429 = 60.0
			multiplierTimeout = 40.0
			latencyDivider = 50.0
		}

		// - avg_latency_ms / latencyDivider
		score -= float64(avgLatencyMs) / latencyDivider

		// - recent_429 * multiplier429
		score -= float64(recent429) * multiplier429

		// - recent_timeout * multiplierTimeout
		score -= float64(recentTimeout) * multiplierTimeout

		// Warmup penalty for cold nodes in RECOVERY system state (gradual traffic redirection)
		if systemState == "RECOVERY" && exists && state.LastErrorAt != nil {
			timeSinceError := time.Since(*state.LastErrorAt)
			if timeSinceError < 5*time.Minute {
				warmupPenalty := (5*time.Minute - timeSinceError).Seconds() * 0.5
				score -= warmupPenalty
			}
		}

		slog.Debug("[Scheduler] Evaluated node score",
			"node_id", node.NodeID,
			"score", score,
			"success_rate", successRate,
			"affinity", hasAffinity,
			"user_priority", userPriority,
			"queue_depth", queueDepth,
			"recent_429", recent429,
			"recent_timeout", recentTimeout,
		)

		if bestNode == nil || score > bestScore {
			bestNode = &finalCandidates[i]
			bestScore = score
		}
	}

	slog.Info("[Scheduler] Node selection completed",
		"winning_node_id", bestNode.NodeID,
		"account_name", bestNode.AccountName,
		"winning_score", bestScore,
		"total_candidates_evaluated", len(finalCandidates),
		"pool_size", len(nodes),
	)

	return bestNode, nil
}
