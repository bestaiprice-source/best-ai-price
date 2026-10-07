package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// Durable administrative audit log (§18).
//
// Every administrative mutation that changes credentials, routing or pricing must
// leave an append-only record naming the operator, the entity and the change —
// without ever storing raw secrets. Records land in `admin_audit_logs`
// (migration 149). The writer is a package-level default (like the Yunwu notifier
// default) so any call site can audit without extra constructor wiring; a nil
// writer degrades to a structured log line.

const (
	AuditActionProviderCreate     = "ADMIN_PROVIDER_CREATE"
	AuditActionProviderUpdate     = "ADMIN_PROVIDER_UPDATE"
	AuditActionProviderDelete     = "ADMIN_PROVIDER_DELETE"
	AuditActionModelRoutingUpdate = "ADMIN_MODEL_ROUTING_UPDATE"
	AuditActionSettingsUpdate     = "ADMIN_SETTINGS_UPDATE"
)

type AdminAuditWriter struct {
	db *sqlx.DB
}

func NewAdminAuditWriter(db *sql.DB) *AdminAuditWriter {
	if db == nil {
		return nil
	}
	return &AdminAuditWriter{db: sqlx.NewDb(db, "postgres")}
}

// Record appends one audit entry. Best-effort: failures are logged, never
// propagated to the mutation that triggered them.
func (w *AdminAuditWriter) Record(ctx context.Context, action, entityType, entityID, operator string, detail map[string]any) {
	if w == nil || w.db == nil {
		return
	}
	if strings.TrimSpace(operator) == "" {
		operator = "system"
	}
	if detail == nil {
		detail = map[string]any{}
	}
	payload, err := json.Marshal(sanitizeAuditDetail(detail))
	if err != nil {
		payload = []byte("{}")
	}
	writeCtx := ctx
	if writeCtx == nil {
		writeCtx = context.Background()
	}
	if _, err := w.db.ExecContext(writeCtx,
		`INSERT INTO admin_audit_logs (action, entity_type, entity_id, operator, detail, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		action, entityType, entityID, operator, string(payload), time.Now().UTC(),
	); err != nil {
		slog.Warn("[AdminAudit] failed to persist audit record", "action", action, "entity", entityType, "error", err)
	}
}

// sanitizeAuditDetail drops anything that looks like a secret so credentials can
// never leak into the audit log.
func sanitizeAuditDetail(detail map[string]any) map[string]any {
	out := make(map[string]any, len(detail))
	for k, v := range detail {
		switch strings.ToLower(k) {
		case "api_key", "apikey", "token", "access_token", "refresh_token", "setup_token",
			"session_key", "secret", "password", "credentials", "credential", "authorization":
			out[k] = "***"
			continue
		}
		if s, ok := v.(string); ok && len(s) > 0 && looksLikeSecret(s) {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

func looksLikeSecret(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "sk-") ||
		strings.HasPrefix(lower, "sk_") ||
		strings.Contains(lower, "bearer ") ||
		strings.HasPrefix(lower, "eyj") // JWT
}

var (
	defaultAdminAuditMu sync.RWMutex
	defaultAdminAudit   *AdminAuditWriter
)

// SetDefaultAdminAuditWriter installs the process-wide audit writer.
func SetDefaultAdminAuditWriter(db *sql.DB) {
	w := NewAdminAuditWriter(db)
	defaultAdminAuditMu.Lock()
	defaultAdminAudit = w
	defaultAdminAuditMu.Unlock()
}

// GetDefaultAdminAuditWriter returns the process-wide audit writer (may be nil).
func GetDefaultAdminAuditWriter() *AdminAuditWriter {
	defaultAdminAuditMu.RLock()
	defer defaultAdminAuditMu.RUnlock()
	return defaultAdminAudit
}

// AuditAdminAction records an administrative change through the default writer.
// Safe to call when no writer is configured (falls back to a log line).
func AuditAdminAction(ctx context.Context, action, entityType, entityID, operator string, detail map[string]any) {
	w := GetDefaultAdminAuditWriter()
	if w == nil {
		sanitized := sanitizeAuditDetail(detail)
		slog.Info("[AdminAudit] "+action, "entity", entityType, "entity_id", entityID, "operator", operator, "detail", sanitized)
		return
	}
	w.Record(ctx, action, entityType, entityID, operator, detail)
}
