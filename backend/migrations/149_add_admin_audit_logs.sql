-- 149_add_admin_audit_logs.sql
-- Durable append-only audit log for administrative changes (§18): provider
-- credentials, model routing, retail pricing/markups, balance adjustments.
-- Replaces the previous stdout-only "AUDIT:" logging.

CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    action      VARCHAR(64)  NOT NULL,
    entity_type VARCHAR(32)  NOT NULL DEFAULT '',
    entity_id   VARCHAR(64)  NOT NULL DEFAULT '',
    operator    VARCHAR(100) NOT NULL DEFAULT 'system',
    detail      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created_at
    ON admin_audit_logs(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_entity
    ON admin_audit_logs(entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_action
    ON admin_audit_logs(action);
