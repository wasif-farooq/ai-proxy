import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import type { AuditEvent } from '../../types';
import { api } from '../../services/api';

/* ─── State ──────────────────────────────────────────────── */

interface AuditState {
  events: AuditEvent[];
  loading: boolean;
  error: string | null;
  filter: {
    severity: string | null;
    eventType: string | null;
  };
}

const initialState: AuditState = {
  events: [],
  loading: false,
  error: null,
  filter: { severity: null, eventType: null },
};

/* ─── Map backend field names ────────────────────────────── */

export function mapEvent(raw: any): AuditEvent {
  return {
    id: raw.id,
    eventType: raw.event_type ?? raw.eventType,
    severity: raw.severity,
    clientId: raw.client_id ?? raw.clientId ?? null,
    adminId: raw.admin_id ?? raw.adminId ?? null,
    actorType: raw.actor_type ?? raw.actorType ?? 'system',
    action: raw.action,
    resource: raw.resource,
    resourceId: raw.resource_id ?? raw.resourceId ?? null,
    providerId: raw.provider_id ?? raw.providerId ?? null,
    model: raw.model ?? null,
    statusCode: raw.status_code ?? raw.statusCode ?? null,
    latencyMs: raw.latency_ms ?? raw.latencyMs ?? null,
    ipAddress: raw.ip_address ?? raw.ipAddress ?? null,
    timestamp: raw.timestamp ?? raw.created_at,
    beforeState: raw.before_state ?? raw.beforeState ?? null,
    afterState: raw.after_state ?? raw.afterState ?? null,
  };
}

/* ─── Thunks ─────────────────────────────────────────────── */

export const fetchAuditLogs = createAsyncThunk('audit/fetchAll', async (_, { getState }) => {
  const state = (getState() as { audit: AuditState }).audit;
  const params = new URLSearchParams();
  if (state.filter.eventType) params.set('event_type', state.filter.eventType);
  if (state.filter.severity) params.set('severity', state.filter.severity);
  params.set('limit', '100');
  params.set('offset', '0');

  const qs = params.toString();
  const data: any[] = await api.get(`/api/v1/admin/audit-logs${qs ? '?' + qs : ''}`);
  return data.map(mapEvent);
});

/* ─── Slice ──────────────────────────────────────────────── */

const auditSlice = createSlice({
  name: 'audit',
  initialState,
  reducers: {
    setSeverityFilter(state, action) {
      state.filter.severity = action.payload;
    },
    setEventTypeFilter(state, action) {
      state.filter.eventType = action.payload;
    },
    clearFilters(state) {
      state.filter = { severity: null, eventType: null };
    },
    appendEvent(state, action) {
      // Used by WebSocket live updates
      state.events.unshift(action.payload);
      if (state.events.length > 500) state.events.pop();
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(fetchAuditLogs.pending, (state) => { state.loading = true; state.error = null; })
      .addCase(fetchAuditLogs.fulfilled, (state, action) => { state.loading = false; state.events = action.payload; })
      .addCase(fetchAuditLogs.rejected, (state, action) => { state.loading = false; state.error = action.error.message ?? 'Failed to fetch audit logs'; });
  },
});

export const { setSeverityFilter, setEventTypeFilter, clearFilters, appendEvent } = auditSlice.actions;

/* ─── Selectors ──────────────────────────────────────────── */

export const selectAuditEvents = (state: { audit: AuditState }) => state.audit.events;
export const selectAuditLoading = (state: { audit: AuditState }) => state.audit.loading;
export const selectAuditError = (state: { audit: AuditState }) => state.audit.error;
export const selectAuditFilter = (state: { audit: AuditState }) => state.audit.filter;

export default auditSlice.reducer;
