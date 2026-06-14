import { useCallback } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import type { AppDispatch } from '../store';
import {
  fetchClients,
  createClient,
  updateClient,
  rotateKeys,
  updateClientStatus,
  selectClient,
  clearSelection,
  clearSecrets,
  selectClients,
  selectClientsLoading,
  selectClientsError,
  selectSelectedClient,
  selectCreateSecret,
  selectRotateSecret,
  selectCreateClientId,
  selectCreateEncryptionKey,
  selectCreateEncryptionSecret,
  selectRotateClientId,
  selectRotateEncryptionKey,
  selectRotateEncryptionSecret,
} from '../store/slices/clientsSlice';
import type { CreateClientRequest, ClientStatus, PreferredRoute } from '../types';

export const useClients = () => {
  const dispatch = useDispatch<AppDispatch>();

  const clients = useSelector(selectClients);
  const loading = useSelector(selectClientsLoading);
  const error = useSelector(selectClientsError);
  const selectedClient = useSelector(selectSelectedClient);
  const createSecret = useSelector(selectCreateSecret);
  const rotateSecret = useSelector(selectRotateSecret);
  const createClientId = useSelector(selectCreateClientId);
  const createEncryptionKey = useSelector(selectCreateEncryptionKey);
  const createEncryptionSecret = useSelector(selectCreateEncryptionSecret);
  const rotateClientId = useSelector(selectRotateClientId);
  const rotateEncryptionKey = useSelector(selectRotateEncryptionKey);
  const rotateEncryptionSecret = useSelector(selectRotateEncryptionSecret);

  const handleFetch = useCallback(() => dispatch(fetchClients()), [dispatch]);
  const handleCreate = useCallback((data: CreateClientRequest) => dispatch(createClient(data)), [dispatch]);
  const handleUpdate = useCallback(
    (id: string, data: { name?: string; status?: ClientStatus; preferred_providers?: PreferredRoute[] }) =>
      dispatch(updateClient({ id, data })),
    [dispatch],
  );
  const handleRotate = useCallback((clientId: string) => dispatch(rotateKeys(clientId)), [dispatch]);
  const handleStatusUpdate = useCallback((id: string, status: ClientStatus) => dispatch(updateClientStatus({ id, status })), [dispatch]);
  const handleSelect = useCallback((id: string | null) => dispatch(id ? selectClient(id) : clearSelection()), [dispatch]);
  const handleClearSecrets = useCallback(() => dispatch(clearSecrets()), [dispatch]);

  return {
    clients,
    loading,
    error,
    selectedClient,
    createSecret,
    rotateSecret,
    createClientId,
    createEncryptionKey,
    createEncryptionSecret,
    rotateClientId,
    rotateEncryptionKey,
    rotateEncryptionSecret,
    fetchClients: handleFetch,
    createClient: handleCreate,
    updateClient: handleUpdate,
    rotateKeys: handleRotate,
    updateClientStatus: handleStatusUpdate,
    selectClient: handleSelect,
    clearSecrets: handleClearSecrets,
  };
};
