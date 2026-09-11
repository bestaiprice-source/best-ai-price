-- 145_add_payment_provider_instances.sql
-- Create payment_provider_instances table for gateway instances (NOWPayments, Stripe, etc.)

CREATE TABLE IF NOT EXISTS payment_provider_instances (
    id BIGSERIAL PRIMARY KEY,
    provider_key VARCHAR(30) NOT NULL,
    name VARCHAR(100) NOT NULL DEFAULT '',
    config TEXT NOT NULL DEFAULT '',
    supported_types VARCHAR(200) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    payment_mode VARCHAR(20) NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    limits TEXT NOT NULL DEFAULT '',
    refund_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    allow_user_refund BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS paymentproviderinstance_provider_key
    ON payment_provider_instances(provider_key);

CREATE INDEX IF NOT EXISTS paymentproviderinstance_enabled
    ON payment_provider_instances(enabled);
