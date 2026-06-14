/**
 * API client — central HTTP layer for all backend calls.
 *
 * The Gin backend wraps all responses in the envelope:
 *   { success: bool, data?: T, error?: { code, message, detail }, meta?: { total, page, limit } }
 *
 * This client unwraps the envelope and throws on errors.
 */

import type { DashboardStats } from '../types';

// Use relative URLs by default (same origin as the page).
// Set VITE_API_URL in .env to override for cross-origin dev setups.
const BASE_URL = import.meta.env.VITE_API_URL ?? '';

interface RequestOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
}

/* ─── Raw envelope from the Gin backend ──────────────────── */

interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  error?: {
    code: number;
    message: string;
    detail?: string;
  };
  meta?: {
    total?: number;
    page?: number;
    limit?: number;
  };
}

/* ─── Error class ────────────────────────────────────────── */

export class ApiError extends Error {
  code: number;
  detail: string;

  constructor(code: number, message: string, detail?: string) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.detail = detail ?? '';
  }
}

/* ─── Request helper ─────────────────────────────────────── */

let authToken: string | null = null;

export function setAuthToken(token: string | null) {
  authToken = token;
  if (token) {
    localStorage.setItem('auth_token', token);
  } else {
    localStorage.removeItem('auth_token');
  }
}

export function getAuthToken(): string | null {
  if (!authToken) {
    authToken = localStorage.getItem('auth_token');
  }
  return authToken;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const token = getAuthToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...opts.headers,
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });

  // Handle 204 No Content
  if (res.status === 204) {
    return undefined as T;
  }

  const envelope: ApiEnvelope<T> = await res.json();

  if (!envelope.success || envelope.error) {
    throw new ApiError(
      envelope.error?.code ?? res.status,
      envelope.error?.message ?? 'Request failed',
      envelope.error?.detail,
    );
  }

  // Return data with meta attached (for paginated responses)
  if (envelope.meta) {
    const result = envelope.data as any;
    if (Array.isArray(result)) {
      (result as any).meta = envelope.meta;
    }
    return result as T;
  }

  return envelope.data as T;
}

/* ─── Public API methods ─────────────────────────────────── */

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),

  // Type-specific helpers
  login: (email: string, password: string) =>
    api.post<{ token: string; admin_id: string; email: string; name: string; role: string; expires_at: number }>(
      '/api/v1/admin/auth/login',
      { email, password },
    ),

  getMe: () =>
    api.get<{ admin_id: string; email: string; name: string; role: string }>('/api/v1/admin/me'),

  getDashboardStats: () =>
    api.get<DashboardStats>('/api/v1/admin/dashboard/stats'),
};

