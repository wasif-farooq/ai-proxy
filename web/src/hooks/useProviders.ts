import { useCallback } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import type { AppDispatch } from '../store';
import {
  fetchProviders,
  createProvider,
  updateProvider,
  deleteProvider,
  selectProviders,
  selectProvidersLoading,
  selectProvidersError,
} from '../store/slices/providersSlice';

export const useProviders = () => {
  const dispatch = useDispatch<AppDispatch>();

  const providers = useSelector(selectProviders);
  const loading = useSelector(selectProvidersLoading);
  const error = useSelector(selectProvidersError);

  const handleFetch = useCallback(() => dispatch(fetchProviders()), [dispatch]);
  const handleCreate = useCallback(
    (input: { provider_id: string; name: string; api_key: string; base_url?: string; models?: string[] }) =>
      dispatch(createProvider(input)),
    [dispatch],
  );
  const handleUpdate = useCallback(
    (id: string, data: { name?: string; api_key?: string; base_url?: string; enabled?: boolean; models?: string[] }) =>
      dispatch(updateProvider({ id, data })),
    [dispatch],
  );
  const handleDelete = useCallback((id: string) => dispatch(deleteProvider(id)), [dispatch]);

  return {
    providers,
    loading,
    error,
    fetchProviders: handleFetch,
    createProvider: handleCreate,
    updateProvider: handleUpdate,
    deleteProvider: handleDelete,
  };
};
