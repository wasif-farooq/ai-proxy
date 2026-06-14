/* ─── Client ─────────────────────────────────────────────── */

export type ClientStatus = 'active' | 'suspended' | 'revoked';

export interface PreferredRoute {
  provider: string;
  model: string;
}

export interface Client {
  id: string;
  clientId: string;
  name: string;
  status: ClientStatus;
  preferredProviders: PreferredRoute[];
  createdAt: string;
  updatedAt: string;
  lastRotatedAt: string | null;
}

export interface CreateClientRequest {
  name: string;
  preferredProviders?: PreferredRoute[];
}

export interface UpdateClientRequest {
  name?: string;
  status?: ClientStatus;
  preferredProviders?: PreferredRoute[];
}

/* ─── Auth ───────────────────────────────────────────────── */

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role: 'super_admin' | 'admin' | 'viewer';
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface AuthState {
  user: AuthUser | null;
  token: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
}

/* ─── Provider ───────────────────────────────────────────── */

export interface Provider {
  id: string;
  providerId: string;
  name: string;
  baseUrl: string;
  enabled: boolean;
  models: string[];
  createdAt: string;
  updatedAt: string;
}

/* ─── Audit ──────────────────────────────────────────────── */

export type AuditSeverity = 'info' | 'warning' | 'error' | 'critical';
export type AuditEventType =
  | 'client_created'
  | 'client_updated'
  | 'client_deleted'
  | 'keys_rotated'
  | 'token_issued'
  | 'token_revoked'
  | 'nonce_validation_failed'
  | 'rate_limit_exceeded'
  | 'api_request'
  | 'admin_login'
  | 'admin_logout';

export interface AuditEvent {
  id: string;
  eventType: AuditEventType;
  severity: AuditSeverity;
  clientId: string | null;
  adminId: string | null;
  actorType: 'client' | 'admin' | 'system';
  action: string;
  resource: string;
  resourceId: string | null;
  providerId: string | null;
  model: string | null;
  statusCode: number | null;
  latencyMs: number | null;
  ipAddress: string | null;
  timestamp: string;
  beforeState: Record<string, string> | null;
  afterState: Record<string, string> | null;
}

/* ─── Connection (live WebSocket) ────────────────────────── */

export interface LiveConnection {
  id: string;
  clientId: string;
  clientName: string;
  provider: string;
  model: string;
  connectedAt: string;
  ipAddress: string;
}

/* ─── Client Provider Keys ────────────────────────────────── */

export interface ClientProviderKeyListItem {
  provider: string;
  hasKey: boolean;
  baseUrl?: string | null;
  models?: string[] | null;
  createdAt: string;
  updatedAt: string;
}

export interface SetClientProviderKeyResponse {
  provider_key: {
    id: string;
    client_id: string;
    provider: string;
    base_url?: string | null;
    models?: string[] | null;
    created_at: string;
    updated_at: string;
  };
  api_key: string;
  warning: string;
}

/* ─── Dashboard stats ────────────────────────────────────── */

export interface DashboardStats {
  totalClients: number;
  activeClients: number;
  totalRequests: number;
  avgLatencyMs: number;
  errorRate: number;
  activeConnections: number;
}
