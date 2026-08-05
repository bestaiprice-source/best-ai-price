package compiler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/observability"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/session"
	"github.com/Wei-Shaw/sub2api/pkg/hypervisor/storage"
)

type SummaryWorker struct {
	store *storage.Storage
	sm    *session.SessionManager
}

func NewSummaryWorker(store *storage.Storage) *SummaryWorker {
	var sm *session.SessionManager
	if store != nil {
		sm = session.NewSessionManager(store)
	}
	return &SummaryWorker{store: store, sm: sm}
}

// ShouldSummarize checks if history length exceeds thresholds (>20 messages or >8000 chars)
func (sw *SummaryWorker) ShouldSummarize(execCtx ExecutionContext) bool {
	if len(execCtx.RecentMessages) > 20 {
		return true
	}
	totalChars := 0
	for _, msg := range execCtx.RecentMessages {
		totalChars += len(msg.Content)
	}
	return totalChars > 8000
}

// GenerateSummary creates a concise summary representation of old messages
func (sw *SummaryWorker) GenerateSummary(execCtx ExecutionContext) string {
	if len(execCtx.RecentMessages) == 0 {
		return execCtx.Summary
	}

	var sb strings.Builder
	if execCtx.Summary != "" {
		sb.WriteString(execCtx.Summary)
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("- Conversation history includes %d total messages.", len(execCtx.RecentMessages)))
	if len(execCtx.RecentMessages) > 0 {
		lastMsg := execCtx.RecentMessages[len(execCtx.RecentMessages)-1]
		preview := lastMsg.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf(" Last role: %s. Preview: %s", lastMsg.Role, preview))
	}

	return sb.String()
}

// TriggerAsyncSummary runs background context summarization without blocking user requests
func (sw *SummaryWorker) TriggerAsyncSummary(ctx context.Context, ownerID string, sessionID string, execCtx ExecutionContext) {
	if !sw.ShouldSummarize(execCtx) {
		return
	}
	observability.Default.SummaryJob()

	slog.Info("[SummaryWorker] Triggering async conversation summarization", "session_id", sessionID, "message_count", len(execCtx.RecentMessages))

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		summaryText := sw.GenerateSummary(execCtx)
		if sw.sm != nil {
			if sess, err := sw.sm.GetSession(bgCtx, sessionID); err == nil && sess != nil {
				if sess.Context == nil {
					sess.Context = &session.ExecutionContext{}
				}
				sess.Context.Summary = summaryText
				sess.UpdatedAt = time.Now()
				if err := sw.sm.SaveSession(bgCtx, sess); err != nil {
					slog.Error("[SummaryWorker] Failed to persist summary", "session_id", sessionID, "error", err)
					return
				}
			}
		}

		// Log SummaryUpdated event
		if sw.store != nil {
			_, err := sw.store.LogEvent(bgCtx, ownerID, &sessionID, nil, "SummaryUpdated", map[string]any{
				"summary_length": len(summaryText),
				"message_count":  len(execCtx.RecentMessages),
			})
			if err != nil {
				slog.Error("[SummaryWorker] Failed to write SummaryUpdated event", "session_id", sessionID, "error", err)
			}
		}

		slog.Info("[SummaryWorker] Completed async conversation summarization", "session_id", sessionID, "summary_length", len(summaryText))
	}()
}
