-- 147_add_cheaper_provider_opportunities.sql
-- Stores discovered cheaper upstream provider opportunities for models

CREATE TABLE IF NOT EXISTS ash_cheaper_provider_opportunities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_family TEXT NOT NULL DEFAULT 'other',
    canonical_model_id TEXT NOT NULL,
    model_name TEXT NOT NULL,
    current_primary_provider TEXT NOT NULL,
    current_primary_price NUMERIC(15, 6) NOT NULL,
    current_backup_provider TEXT,
    current_backup_price NUMERIC(15, 6),
    candidate_provider TEXT NOT NULL,
    candidate_domain TEXT NOT NULL,
    candidate_price NUMERIC(15, 6) NOT NULL,
    candidate_input_price NUMERIC(15, 6),
    candidate_output_price NUMERIC(15, 6),
    diff_percent NUMERIC(10, 2) NOT NULL,
    billing_unit TEXT NOT NULL DEFAULT 'tokens',
    provider_url TEXT,
    provider_card_url TEXT,
    status TEXT NOT NULL DEFAULT 'detected', -- 'detected', 'promoted_primary', 'promoted_backup', 'dismissed'
    telegram_sent BOOLEAN NOT NULL DEFAULT false,
    discovered_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (canonical_model_id, candidate_domain)
);

CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_status ON ash_cheaper_provider_opportunities(status);
CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_family ON ash_cheaper_provider_opportunities(model_family);
CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_model ON ash_cheaper_provider_opportunities(canonical_model_id);

