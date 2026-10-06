-- 148_add_model_family_to_cheaper_opportunities.sql
-- Adds model_family column and index to ash_cheaper_provider_opportunities

ALTER TABLE ash_cheaper_provider_opportunities 
ADD COLUMN IF NOT EXISTS model_family TEXT NOT NULL DEFAULT 'other';

CREATE INDEX IF NOT EXISTS idx_ash_cheaper_opportunities_family 
ON ash_cheaper_provider_opportunities(model_family);
