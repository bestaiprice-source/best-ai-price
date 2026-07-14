-- 135_add_ash_hypervisor_tables.sql
-- Create ash_events and ash_usages tables

CREATE TABLE IF NOT EXISTS ash_events (
    id BIGSERIAL PRIMARY KEY,
    owner_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64),
    node_id VARCHAR(64),
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ash_usages (
    id BIGSERIAL PRIMARY KEY,
    owner_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(64),
    node_id VARCHAR(64),
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cost_units BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
