CREATE TABLE IF NOT EXISTS ash_user_policies (
    id BIGSERIAL PRIMARY KEY,
    owner_id VARCHAR(64) NOT NULL,
    pool_type VARCHAR(32) NOT NULL,
    plan_id VARCHAR(64) NOT NULL DEFAULT 'pool-basic',
    monthly_limit BIGINT NOT NULL DEFAULT 0 CHECK (monthly_limit >= 0),
    daily_burst BIGINT NOT NULL DEFAULT 0 CHECK (daily_burst >= 0),
    monthly_limit_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (monthly_limit_micro_usd >= 0),
    daily_burst_micro_usd BIGINT NOT NULL DEFAULT 0 CHECK (daily_burst_micro_usd >= 0),
    priority INTEGER NOT NULL DEFAULT 1 CHECK (priority > 0),
    deposit_state VARCHAR(32) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ash_user_policies_owner_pool_key UNIQUE (owner_id, pool_type)
);
