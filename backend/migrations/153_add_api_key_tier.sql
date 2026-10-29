-- 153_add_api_key_tier.sql
-- Billing tier of an API key for unofficial (yunwu) providers.
--
-- A provider issues its keys inside two billing groups — `smart` (default) and
-- `fast` — which differ in speed and price. The tier used to be derived from the
-- NAMES of the groups the key was bound to, which meant the choice only existed
-- when the administrator had created groups literally named `smart` / `fast`.
-- The tier is now an attribute of the key itself:
--   * ''      -> not set: the tier is derived from the key's group names, so
--                keys created before this column (and group-based deployments)
--                keep behaving exactly as before;
--   * 'smart' -> the provider's primary (cheaper) key is used;
--   * 'fast'  -> the provider's dedicated fast key is used and the fast group's
--                price is charged.
--
-- Additive only: existing rows keep working (NOT NULL with a '' default), and
-- nothing reads this column unless it is set.

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS tier VARCHAR(16) NOT NULL DEFAULT '';
