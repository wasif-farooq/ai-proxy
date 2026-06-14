import { useCallback } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import type { AppDispatch } from '../store';
import {
  fetchAuditLogs,
  setSeverityFilter,
  setEventTypeFilter,
  clearFilters,
  selectAuditEvents,
  selectAuditLoading,
  selectAuditError,
  selectAuditFilter,
} from '../store/slices/auditSlice';

export const useAuditLogs = () => {
  const dispatch = useDispatch<AppDispatch>();

  const events = useSelector(selectAuditEvents);
  const loading = useSelector(selectAuditLoading);
  const error = useSelector(selectAuditError);
  const filter = useSelector(selectAuditFilter);

  const handleFetch = useCallback(() => dispatch(fetchAuditLogs()), [dispatch]);
  const handleSeverityFilter = useCallback((sev: string | null) => dispatch(setSeverityFilter(sev)), [dispatch]);
  const handleEventTypeFilter = useCallback((type: string | null) => dispatch(setEventTypeFilter(type)), [dispatch]);
  const handleClearFilters = useCallback(() => dispatch(clearFilters()), [dispatch]);

  return {
    events,
    loading,
    error,
    filter,
    fetchAuditLogs: handleFetch,
    setSeverityFilter: handleSeverityFilter,
    setEventTypeFilter: handleEventTypeFilter,
    clearFilters: handleClearFilters,
  };
};
