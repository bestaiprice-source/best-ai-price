-- 136_add_ash_sessions_table.sql
-- Create ash_sessions table for hypervisor state

CREATE TABLE IF NOT EXISTS ash_sessions (
    id VARCHAR(64) PRIMARY KEY,
    owner_id VARCHAR(64) NOT NULL,
    conversation_id VARCHAR(64) NOT NULL,
    state VARCHAR(32) NOT NULL,
    summary TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ash_sessions_owner ON ash_sessions (owner_id);
CREATE INDEX IF NOT EXISTS idx_ash_sessions_conversation ON ash_sessions (conversation_id);
