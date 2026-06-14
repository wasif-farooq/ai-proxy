CREATE TABLE IF NOT EXISTS client_provider_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id   UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    provider    VARCHAR(64) NOT NULL,
    api_key     TEXT NOT NULL,                     -- encrypted at rest
    base_url    VARCHAR(500),                      -- optional per-client base URL override
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(client_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_client_provider_keys_client_id ON client_provider_keys(client_id);
