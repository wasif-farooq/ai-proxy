CREATE TABLE IF NOT EXISTS clients (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id           VARCHAR(64) UNIQUE NOT NULL,
    client_secret_hash  VARCHAR(255) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    status              VARCHAR(20) DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'revoked')),
    encryption_key      VARCHAR(255) NOT NULL,
    encryption_secret   VARCHAR(255) NOT NULL,
    preferred_providers JSONB DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    last_rotated_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_clients_client_id ON clients(client_id);
CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(status);
