CREATE TABLE IF NOT EXISTS audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type   VARCHAR(50) NOT NULL,
    severity     VARCHAR(20) NOT NULL CHECK (severity IN ('info', 'warning', 'error', 'critical')),

    client_id    UUID REFERENCES clients(id) ON DELETE SET NULL,
    admin_id     UUID REFERENCES admin_users(id) ON DELETE SET NULL,
    actor_type   VARCHAR(20) NOT NULL CHECK (actor_type IN ('client', 'admin', 'system')),

    request_id   VARCHAR(64),
    ip_address   INET,
    user_agent   TEXT,

    action       VARCHAR(100) NOT NULL,
    resource     VARCHAR(100) NOT NULL,
    resource_id  VARCHAR(255),

    provider_id  VARCHAR(64),
    model        VARCHAR(100),
    status_code  INT,
    latency_ms   INT,

    nonce_valid  BOOLEAN,
    token_type   VARCHAR(20),

    before_state JSONB,
    after_state  JSONB,

    timestamp    TIMESTAMPTZ DEFAULT NOW(),
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_client_id  ON audit_logs(client_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_admin_id   ON audit_logs(admin_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type ON audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp  ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_severity   ON audit_logs(severity);
