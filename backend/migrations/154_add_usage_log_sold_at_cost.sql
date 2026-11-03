-- 154_add_usage_log_sold_at_cost.sql
-- Marks a usage row that was billed at the SERVING provider's own cost.
--
-- The published (retail) price is derived from the primary — cheapest — provider
-- for a model. When the request is actually served by a more expensive provider,
-- that price would sell the request below its own cost. The per-request loss
-- observer now re-prices such a request at the serving provider's rate, keeping the
-- normal margin (see CostBreakdown.SoldAtCost / observeYunwuRequestCost). This
-- column records that the row was corrected, so such requests are visible in the
-- usage history and not only in the Telegram alert.
--
-- Additive only: default false, existing rows are unaffected and keep reporting
-- as before.

ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS sold_at_cost BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_usage_logs_sold_at_cost ON usage_logs (sold_at_cost) WHERE sold_at_cost;
