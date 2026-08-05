ALTER TABLE ash_usages ADD COLUMN IF NOT EXISTS pool_type VARCHAR(32) NOT NULL DEFAULT 'legacy';
ALTER TABLE ash_usages ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT 'legacy';
ALTER TABLE ash_usages ADD COLUMN IF NOT EXISTS model VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE ash_usages ADD COLUMN IF NOT EXISTS usage_unit VARCHAR(32) NOT NULL DEFAULT 'tokens';
ALTER TABLE ash_usages ADD COLUMN IF NOT EXISTS billed_micro_usd BIGINT NOT NULL DEFAULT 0;

UPDATE ash_usages SET pool_type='legacy', provider='legacy', usage_unit='tokens'
WHERE pool_type IS NULL OR pool_type = '';

CREATE INDEX IF NOT EXISTS idx_ash_usages_owner_pool_month
ON ash_usages(owner_id, pool_type, created_at);
