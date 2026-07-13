package storage

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
)

// LogEvent writes an append-only event to the ash_events table
func (s *Storage) LogEvent(ctx context.Context, ownerID string, sessionID, nodeID *string, eventType string, payload map[string]any) (*ent.AshEvent, error) {
	builder := s.db.AshEvent.Create().
		SetOwnerID(ownerID).
		SetEventType(eventType).
		SetPayload(payload)

	if sessionID != nil {
		builder.SetSessionID(*sessionID)
	}
	if nodeID != nil {
		builder.SetNodeID(*nodeID)
	}

	return builder.Save(ctx)
}

// RecordUsage writes usage details to the ash_usages table
func (s *Storage) RecordUsage(ctx context.Context, ownerID string, sessionID, nodeID *string, inputTokens, outputTokens, costUnits int64) (*ent.AshUsage, error) {
	builder := s.db.AshUsage.Create().
		SetOwnerID(ownerID).
		SetInputTokens(inputTokens).
		SetOutputTokens(outputTokens).
		SetCostUnits(costUnits)

	if sessionID != nil {
		builder.SetSessionID(*sessionID)
	}
	if nodeID != nil {
		builder.SetNodeID(*nodeID)
	}

	return builder.Save(ctx)
}
