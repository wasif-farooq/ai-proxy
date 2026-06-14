import { useEffect } from 'react';
import { useClients } from '../hooks/useClients';
import { useDashboard } from '../hooks/useDashboard';
import { Card, Skeleton } from '../components/common';
import type { DashboardStats } from '../types';

const statCards: { label: string; key: keyof DashboardStats; format: (v: number) => string }[] = [
  { label: 'Total Clients', key: 'totalClients', format: (v: number) => v.toString() },
  { label: 'Active Clients', key: 'activeClients', format: (v: number) => v.toString() },
  { label: 'Requests (30d)', key: 'totalRequests', format: (v: number) => v.toLocaleString() },
  { label: 'Avg Latency', key: 'avgLatencyMs', format: (v: number) => `${v}ms` },
  { label: 'Error Rate', key: 'errorRate', format: (v: number) => `${v}%` },
  { label: 'Active Providers', key: 'activeConnections', format: (v: number) => v.toString() },
];

export const Dashboard = () => {
  const { stats, loading: statsLoading, error: statsError } = useDashboard();
  const { clients, fetchClients, loading: clientsLoading } = useClients();

  useEffect(() => {
    fetchClients();
  }, [fetchClients]);

  const recentClients = clients.slice(0, 4);

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      <h1 className="text-ink text-[28px] font-[700] leading-[1.43] mb-1">Dashboard</h1>
      <p className="text-muted text-sm font-[400] leading-[1.43] mb-8">
        Overview of your AI proxy infrastructure
      </p>

      {/* Stats error banner */}
      {statsError && (
        <div className="bg-red-50 border border-red-200 rounded-xs px-4 py-3 text-sm text-primary-error mb-4">
          Failed to load stats: {statsError}
        </div>
      )}

      {/* Metric cards grid */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4 mb-10">
        {statCards.map((stat) => (
          <Card key={stat.key} variant="host" className="text-center">
            <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
              {stat.label}
            </p>
            {statsLoading ? (
              <Skeleton width="w-1/2" className="mx-auto" />
            ) : (
              <p className="text-ink text-[22px] font-[500] leading-[1.18] -tracking-[0.44px]">
                {stat.format(stats[stat.key])}
              </p>
            )}
          </Card>
        ))}
      </div>

      {/* Recent clients */}
      <h2 className="text-ink text-[20px] font-[600] leading-[1.2] -tracking-[0.18px] mb-4">
        Recent Clients
      </h2>
      {clientsLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i} variant="host">
              <Skeleton lines={3} />
            </Card>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {recentClients.map((client) => {
            const statusColor: Record<string, string> = {
              active: 'bg-green-500',
              suspended: 'bg-amber-500',
              revoked: 'bg-red-500',
            };
            return (
              <Card key={client.id} variant="host" hoverable>
                <div className="flex items-center gap-2 mb-2">
                  <span className={`w-2 h-2 rounded-full ${statusColor[client.status] ?? 'bg-muted'}`} />
                  <span className="text-ink text-sm font-[600] leading-[1.25]">{client.name}</span>
                </div>
                <p className="text-muted text-[13px] font-[400] leading-[1.23]">
                  {client.clientId}
                </p>
                <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mt-1">
                  {client.preferredProviders.join(', ')}
                </p>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
};
