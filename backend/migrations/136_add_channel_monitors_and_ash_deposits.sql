-- 136_add_channel_monitors_and_ash_deposits.sql
-- Create ash_deposits, channel_monitor_request_templates, channel_monitors, channel_monitor_daily_rollups, channel_monitor_histories

CREATE TABLE IF NOT EXISTS ash_deposits (
    id BIGSERIAL PRIMARY KEY,
    owner_id VARCHAR(64) NOT NULL,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(8) NOT NULL DEFAULT 'USD',
    state VARCHAR(32) NOT NULL DEFAULT 'held',
    notice_days INT NOT NULL DEFAULT 10,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ashdeposit_owner_id ON ash_deposits(owner_id);
CREATE INDEX IF NOT EXISTS ashdeposit_state ON ash_deposits(state);

CREATE TABLE IF NOT EXISTS channel_monitor_request_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    preset_key VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider VARCHAR(50) NOT NULL,
    endpoint VARCHAR(500) NOT NULL,
    primary_model VARCHAR(200) NOT NULL,
    extra_models JSONB NOT NULL DEFAULT '[]',
    interval_seconds INT NOT NULL DEFAULT 60,
    extra_headers JSONB NOT NULL DEFAULT '{}',
    body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    body_override JSONB
);

CREATE TABLE IF NOT EXISTS channel_monitors (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name VARCHAR(100) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    endpoint VARCHAR(500) NOT NULL,
    api_key_encrypted TEXT NOT NULL,
    primary_model VARCHAR(200) NOT NULL,
    extra_models JSONB NOT NULL DEFAULT '[]',
    group_name VARCHAR(100) DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    interval_seconds INT NOT NULL DEFAULT 60,
    last_checked_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL DEFAULT 0,
    extra_headers JSONB NOT NULL DEFAULT '{}',
    body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    body_override JSONB,
    template_id BIGINT REFERENCES channel_monitor_request_templates(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS channel_monitor_daily_rollups (
    id BIGSERIAL PRIMARY KEY,
    date DATE NOT NULL,
    total_checks INT NOT NULL DEFAULT 0,
    success_checks INT NOT NULL DEFAULT 0,
    failed_checks INT NOT NULL DEFAULT 0,
    total_latency_ms BIGINT NOT NULL DEFAULT 0,
    min_latency_ms INT NOT NULL DEFAULT 0,
    max_latency_ms INT NOT NULL DEFAULT 0,
    p95_latency_ms INT NOT NULL DEFAULT 0,
    status_2xx_count INT NOT NULL DEFAULT 0,
    status_4xx_count INT NOT NULL DEFAULT 0,
    status_5xx_count INT NOT NULL DEFAULT 0,
    other_error_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    monitor_id BIGINT NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS channel_monitor_histories (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status_code INT NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    response_snippet TEXT NOT NULL DEFAULT '',
    tested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    monitor_id BIGINT NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE
);
