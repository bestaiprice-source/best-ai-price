-- 087_add_channels_tables.sql
-- Create channels and all related tables

CREATE TABLE IF NOT EXISTS channels (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    model_mapping JSONB NOT NULL DEFAULT '{}',
    billing_model_source VARCHAR(50) NOT NULL DEFAULT 'requested',
    restrict_models BOOLEAN NOT NULL DEFAULT FALSE,
    features TEXT NOT NULL DEFAULT '',
    features_config JSONB NOT NULL DEFAULT '{}',
    apply_pricing_to_account_stats BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_groups (
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (channel_id, group_id)
);

CREATE TABLE IF NOT EXISTS channel_model_pricing (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL DEFAULT 'anthropic',
    models JSONB NOT NULL DEFAULT '[]',
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'token',
    input_price DECIMAL(20,8),
    output_price DECIMAL(20,8),
    cache_write_price DECIMAL(20,8),
    cache_read_price DECIMAL(20,8),
    image_output_price DECIMAL(20,8),
    per_request_price DECIMAL(20,8),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_pricing_intervals (
    id BIGSERIAL PRIMARY KEY,
    pricing_id BIGINT NOT NULL REFERENCES channel_model_pricing(id) ON DELETE CASCADE,
    min_tokens INT NOT NULL DEFAULT 0,
    max_tokens INT,
    tier_label VARCHAR(100) NOT NULL DEFAULT '',
    input_price DECIMAL(20,8),
    output_price DECIMAL(20,8),
    cache_write_price DECIMAL(20,8),
    cache_read_price DECIMAL(20,8),
    per_request_price DECIMAL(20,8),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_rules (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL DEFAULT '',
    group_ids BIGINT[],
    account_ids BIGINT[],
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_account_stats_model_pricing (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES channel_account_stats_pricing_rules(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL DEFAULT '',
    models JSONB NOT NULL DEFAULT '[]',
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'token',
    input_price DECIMAL(20,8),
    output_price DECIMAL(20,8),
    cache_write_price DECIMAL(20,8),
    cache_read_price DECIMAL(20,8),
    image_output_price DECIMAL(20,8),
    per_request_price DECIMAL(20,8),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_intervals (
    id BIGSERIAL PRIMARY KEY,
    pricing_id BIGINT NOT NULL REFERENCES channel_account_stats_model_pricing(id) ON DELETE CASCADE,
    min_tokens INT NOT NULL DEFAULT 0,
    max_tokens INT,
    tier_label VARCHAR(100) NOT NULL DEFAULT '',
    input_price DECIMAL(20,8),
    output_price DECIMAL(20,8),
    cache_write_price DECIMAL(20,8),
    cache_read_price DECIMAL(20,8),
    per_request_price DECIMAL(20,8),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
