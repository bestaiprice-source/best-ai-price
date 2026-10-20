-- Adds an optional, encrypted copy of the API key so its owner can reveal it later.
--
-- Backward/forward compatible:
--   * existing rows keep key_encrypted = NULL and stay hash-only (irrecoverable);
--   * authentication continues to work off api_keys.key (the deterministic hash),
--     so keys created before this migration keep working unchanged;
--   * only keys created after the migration get a recoverable copy.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_encrypted TEXT;

COMMENT ON COLUMN api_keys.key_encrypted IS 'AES-256-GCM encrypted plaintext API key (NULL for legacy hash-only rows; authentication never uses this column).';
