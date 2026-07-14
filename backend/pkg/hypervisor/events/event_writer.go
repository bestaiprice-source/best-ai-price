package events

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type EventWriter struct {
	store *storage.Storage
}

func NewEventWriter(store *storage.Storage) *EventWriter {
	return &EventWriter{
		store: store,
	}
}

// LogEvent delegates event logging to the postgres storage methods
func (ew *EventWriter) LogEvent(ctx context.Context, ownerID string, sessionID, nodeID *string, eventType string, payload map[string]any) (*ent.AshEvent, error) {
	return ew.store.LogEvent(ctx, ownerID, sessionID, nodeID, eventType, payload)
}
