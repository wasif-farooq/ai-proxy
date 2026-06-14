import { useState, useCallback, useEffect } from 'react';
import type { DashboardStats } from '../types';
import { api } from '../services/api';

const EMPTY_STATS: DashboardStats = {
  totalClients: 0,
  activeClients: 0,
  totalRequests: 0,
  avgLatencyMs: 0,
  errorRate: 0,
  activeConnections: 0,
};

export const useDashboard = () => {
  const [stats, setStats] = useState<DashboardStats>(EMPTY_STATS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStats = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data: any = await api.getDashboardStats();
      setStats({
        totalClients: data.total_clients ?? data.totalClients ?? 0,
        activeClients: data.active_clients ?? data.activeClients ?? 0,
        totalRequests: data.total_requests_30d ?? data.totalRequests ?? 0,
        avgLatencyMs: data.avg_latency_ms ?? data.avgLatencyMs ?? 0,
        errorRate: data.error_rate ?? data.errorRate ?? 0,
        activeConnections: data.active_providers ?? data.activeConnections ?? 0,
      });
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  return { stats, loading, error, refetch: fetchStats };
};
