ALTER TABLE client_provider_keys
ADD COLUMN IF NOT EXISTS models JSONB;

COMMENT ON COLUMN client_provider_keys.models IS 'Optional list of allowed models for this per-client key. NULL or empty means all models are allowed.';
