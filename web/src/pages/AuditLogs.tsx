import { useEffect, useCallback } from 'react';
import { useDispatch } from 'react-redux';
import { useAuditLogs } from '../hooks/useAuditLogs';
import { useWebSocket } from '../hooks/useWebSocket';
import { appendEvent, mapEvent } from '../store/slices/auditSlice';
import { Table, Card, Badge, Button } from '../components/common';
import type { AuditEvent, AuditSeverity, AuditEventType } from '../types';

const eventTypeLabels: { value: AuditEventType; label: string }[] = [
  { value: 'client_created', label: 'Client Created' },
  { value: 'client_updated', label: 'Client Updated' },
  { value: 'keys_rotated', label: 'Keys Rotated' },
  { value: 'token_issued', label: 'Token Issued' },
  { value: 'rate_limit_exceeded', label: 'Rate Limit' },
  { value: 'nonce_validation_failed', label: 'Nonce Failed' },
  { value: 'api_request', label: 'API Request' },
  { value: 'admin_login', label: 'Admin Login' },
];

/* ─── Map WS event (reuses slice's mapEvent, adds fallback timestamp) ── */

function mapWsEvent(raw: any): AuditEvent {
  const event = mapEvent(raw);
  if (!event.timestamp) {
    event.timestamp = new Date().toISOString();
  }
  return event;
}

export const AuditLogs = () => {
  const dispatch = useDispatch();

  const {
    events,
    loading,
    fetchAuditLogs,
    filter,
    setSeverityFilter,
    setEventTypeFilter,
    clearFilters,
  } = useAuditLogs();

  useEffect(() => {
    fetchAuditLogs();
  }, [fetchAuditLogs]);

  // WebSocket live connection — dispatch incoming events to Redux
  const handleWsMessage = useCallback(
    (data: unknown) => {
      const event = mapWsEvent(data);
      dispatch(appendEvent(event));
    },
    [dispatch],
  );

  const { connected, retryCount } = useWebSocket({
    url: '/api/v1/admin/ws/connections',
    onMessage: handleWsMessage,
    autoConnect: true,
    maxRetries: 10,
  });

  const columns = [
    {
      key: 'timestamp',
      header: 'Time',
      render: (e: AuditEvent) => (
        <span className="text-ink text-[13px] whitespace-nowrap">
          {new Date(e.timestamp).toLocaleString()}
        </span>
      ),
    },
    {
      key: 'severity',
      header: 'Severity',
      render: (e: AuditEvent) => <Badge variant="status">{e.severity}</Badge>,
    },
    {
      key: 'eventType',
      header: 'Event',
      render: (e: AuditEvent) => (
        <span className="text-ink text-sm font-[500]">
          {e.eventType.replace(/_/g, ' ')}
        </span>
      ),
    },
    {
      key: 'action',
      header: 'Action',
      render: (e: AuditEvent) => (
        <span className="text-muted text-[13px]">{e.action}</span>
      ),
    },
    {
      key: 'client',
      header: 'Client',
      render: (e: AuditEvent) => (
        <span className="text-muted text-[13px]">
          {e.clientId ?? e.actorType}
        </span>
      ),
    },
    {
      key: 'ip',
      header: 'IP Address',
      render: (e: AuditEvent) => (
        <span className="text-muted text-[13px]">{e.ipAddress ?? '—'}</span>
      ),
    },
  ];

  const hasActiveFilter = filter.severity !== null || filter.eventType !== null;

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header with live indicator */}
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-ink text-[28px] font-[700] leading-[1.43]">Audit Logs</h1>
        <div className="flex items-center gap-2">
          {/* Live indicator */}
          <div
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[11px] font-[600] leading-[1.18] transition-colors duration-300 ${
              connected
                ? 'bg-green-50 text-green-700'
                : 'bg-amber-50 text-amber-700'
            }`}
            title={connected ? 'Connected — receiving live events' : `Disconnected — retry ${retryCount}`}
          >
            <span
              className={`w-2 h-2 rounded-full transition-colors duration-300 ${
                connected ? 'bg-green-500 animate-pulse' : 'bg-amber-500'
              }`}
            />
            <span>{connected ? 'Live' : 'Reconnecting…'}</span>
          </div>
        </div>
      </div>
      <p className="text-muted text-sm font-[400] leading-[1.43] mb-8">
        Security and compliance event history
      </p>

      {/* Filters */}
      <Card variant="default" className="mb-6">
        <div className="flex flex-wrap items-center gap-3">
          {/* Severity filter */}
          <div className="flex items-center gap-2">
            <span className="text-muted text-[13px] font-[500]">Severity:</span>
            {(['info', 'warning', 'error', 'critical'] as AuditSeverity[]).map((sev) => (
              <button
                key={sev}
                onClick={() => setSeverityFilter(filter.severity === sev ? null : sev)}
                className={`
                  px-3 py-1.5 rounded-full text-[11px] font-[600] leading-[1.18]
                  transition-colors duration-150 cursor-pointer border-none
                  ${filter.severity === sev
                    ? 'bg-ink text-on-dark'
                    : 'bg-surface-soft text-muted hover:bg-hairline'
                  }
                `}
              >
                {sev}
              </button>
            ))}
          </div>

          <div className="w-px h-6 bg-hairline" />

          {/* Event type filter */}
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-muted text-[13px] font-[500]">Type:</span>
            <select
              value={filter.eventType ?? ''}
              onChange={(e) => setEventTypeFilter(e.target.value || null)}
              className="
                h-9 rounded-sm px-3
                bg-canvas text-ink text-sm font-[400] leading-[1.43]
                border border-hairline
                focus:outline-none focus:border-ink focus:border-2
                cursor-pointer
              "
            >
              <option value="">All events</option>
              {eventTypeLabels.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>

          {hasActiveFilter && (
            <Button variant="tertiary-text" onClick={clearFilters} className="text-[13px]">
              Clear filters
            </Button>
          )}
        </div>
      </Card>

      {/* Table */}
      <Card variant="default" className="p-0">
        <Table
          columns={columns}
          data={events}
          loading={loading}
          emptyMessage="No audit events match the current filters."
          keyExtractor={(e) => e.id}
        />
      </Card>
    </div>
  );
};
