-- Hash service API keys at rest. Authentication performs the same SHA-256
-- transform before lookup; the original cleartext cannot be recovered.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE api_keys
SET key = 'sha256:' || LEFT(key, 8) || ':' || encode(digest(key, 'sha256'), 'hex'),
    updated_at = NOW()
WHERE key NOT LIKE 'sha256:%';

COMMENT ON COLUMN api_keys.key IS
  'Non-reversible sha256:<display-prefix>:<digest>; cleartext is returned only at creation';
