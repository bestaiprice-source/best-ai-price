package health

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type ExecutionNode struct {
	NodeID       string   `json:"node_id"`
	AccountName  string   `json:"account_name"`
	Provider     string   `json:"provider"`
	State        string   `json:"state"` // healthy, busy, limited, unavailable, cooldown
	Capabilities []string `json:"capabilities"`
	Tiers        []string `json:"tiers,omitempty"` // billing tiers the node holds a key for (smart/fast)
	// TierGroups maps a billing tier to the upstream provider billing group that
	// tier's key was issued in (account.Extra smart_group_name/fast_group_name).
	// Empty means "not configured" — the price then follows the legacy
	// derivation. The engine prices a candidate node with its own groups, so a
	// pinned group must travel with the node.
	TierGroups map[string]string `json:"tier_groups,omitempty"`
	Priority   int               `json:"priority"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type NodeRuntimeState struct {
	NodeID               string     `json:"node_id"`
	SuccessRate          float64    `json:"success_rate"`
	AvgLatencyMs         int64      `json:"avg_latency_ms"`
	QueueDepth           int        `json:"queue_depth"`
	Recent429            int        `json:"recent_429"`
	RecentTimeouts       int        `json:"recent_timeouts"`
	UsedTokens           int64      `json:"used_tokens"`
	TotalRequests        int64      `json:"total_requests"`
	SuccessfulRequests   int64      `json:"successful_requests"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	LastSuccessAt        *time.Time `json:"last_success_at,omitempty"`
	LastErrorAt          *time.Time `json:"last_error_at,omitempty"`
	CooldownUntil        *time.Time `json:"cooldown_until,omitempty"`
}

type NodeManager struct {
	store *storage.Storage
}

func NewNodeManager(store *storage.Storage) *NodeManager {
	return &NodeManager{store: store}
}

func NodeRuntimeKey(nodeID string) string {
	return fmt.Sprintf("ash:node:%s:runtime", nodeID)
}

func (nm *NodeManager) GetNode(ctx context.Context, nodeID string) (*ExecutionNode, error) {
	key := storage.NodeKey(nodeID)
	data, err := nm.store.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var node ExecutionNode
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (nm *NodeManager) SaveNode(ctx context.Context, node *ExecutionNode) error {
	key := storage.NodeKey(node.NodeID)
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	return nm.store.Redis().Set(ctx, key, data, 0).Err()
}

func (nm *NodeManager) DeleteNode(ctx context.Context, nodeID string) error {
	key := storage.NodeKey(nodeID)
	return nm.store.Redis().Del(ctx, key).Err()
}

func (nm *NodeManager) GetNodeRuntimeState(ctx context.Context, nodeID string) (*NodeRuntimeState, error) {
	key := NodeRuntimeKey(nodeID)
	data, err := nm.store.Redis().Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var state NodeRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (nm *NodeManager) SaveNodeRuntimeState(ctx context.Context, state *NodeRuntimeState) error {
	key := NodeRuntimeKey(state.NodeID)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return nm.store.Redis().Set(ctx, key, data, 0).Err()
}

func (nm *NodeManager) DeleteNodeRuntimeState(ctx context.Context, nodeID string) error {
	key := NodeRuntimeKey(nodeID)
	return nm.store.Redis().Del(ctx, key).Err()
}

func (nm *NodeManager) GetAllNodes(ctx context.Context) ([]ExecutionNode, error) {
	if nm.store == nil || nm.store.Redis() == nil {
		return nil, nil
	}
	var cursor uint64
	var allNodes []ExecutionNode
	for {
		keys, nextCursor, err := nm.store.Redis().Scan(ctx, cursor, "ash:node:*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if len(key) > 8 && key[len(key)-8:] == ":runtime" {
				continue
			}
			data, err := nm.store.Redis().Get(ctx, key).Bytes()
			if err != nil {
				continue
			}
			var node ExecutionNode
			if err := json.Unmarshal(data, &node); err == nil {
				allNodes = append(allNodes, node)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return allNodes, nil
}
