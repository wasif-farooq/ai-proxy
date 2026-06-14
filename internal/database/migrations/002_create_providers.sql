CREATE TABLE IF NOT EXISTS providers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id VARCHAR(64) UNIQUE NOT NULL,
    name        VARCHAR(255) NOT NULL,
    api_key     VARCHAR(255) NOT NULL,
    base_url    VARCHAR(500) NOT NULL,
    enabled     BOOLEAN DEFAULT true,
    models      JSONB DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);
