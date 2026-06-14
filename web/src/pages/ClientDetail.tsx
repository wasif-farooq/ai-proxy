import { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import type { AppDispatch } from '../store';
import {
  fetchClient as fetchClientThunk,
  updateClient,
  rotateKeys,
  updateClientStatus,
  clearSecrets,
  selectClients,
  selectCreateSecret,
  selectRotateSecret,
  selectCreateClientId,
  selectCreateEncryptionKey,
  selectCreateEncryptionSecret,
  selectRotateClientId,
  selectRotateEncryptionKey,
  selectRotateEncryptionSecret,
} from '../store/slices/clientsSlice';
import { Button, Card, Badge, Input, CopyButton, SortableList, Modal } from '../components/common';
import { ProviderKeyList } from '../components/ProviderKeyList';
import { useToast } from '../components/common';
import { useProviders } from '../hooks/useProviders';
import { api } from '../services/api';
import type { Client, ClientProviderKeyListItem, SetClientProviderKeyResponse, PreferredRoute } from '../types';

/* ─── Client Detail Page (2-column layout) ──────────────── */

export const ClientDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const dispatch = useDispatch<AppDispatch>();
  const { addToast } = useToast();
  const { providers, fetchProviders } = useProviders();

  /* ── Find client from store ──────────────────────────── */
  const clients = useSelector(selectClients);
  const client = clients.find((c) => c.id === id) ?? null;

  const createSecret = useSelector(selectCreateSecret);
  const rotateSecret = useSelector(selectRotateSecret);
  const createClientId = useSelector(selectCreateClientId);
  const createEncryptionKey = useSelector(selectCreateEncryptionKey);
  const createEncryptionSecret = useSelector(selectCreateEncryptionSecret);
  const rotateClientId = useSelector(selectRotateClientId);
  const rotateEncryptionKey = useSelector(selectRotateEncryptionKey);
  const rotateEncryptionSecret = useSelector(selectRotateEncryptionSecret);

  /* ── Local state ─────────────────────────────────────── */
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [editRoutes, setEditRoutes] = useState<PreferredRoute[]>([]);
  const [editNewProvider, setEditNewProvider] = useState('');
  const [editNewModel, setEditNewModel] = useState('');
  const [showAddRoute, setShowAddRoute] = useState(false);
  const [confirmRotate, setConfirmRotate] = useState(false);
  const [confirmStatusChange, setConfirmStatusChange] = useState<Client['status'] | null>(null);
  const [confirmSave, setConfirmSave] = useState(false);
  const [secretOpen, setSecretOpen] = useState<'create' | 'rotate' | null>(null);

  /* Provider keys */
  const [providerKeys, setProviderKeys] = useState<ClientProviderKeyListItem[]>([]);
  const [providerKeysLoading, setProviderKeysLoading] = useState(false);
  const [setKeyOpen, setSetKeyOpen] = useState(false);
  const [setKeyProvider, setSetKeyProvider] = useState('');
  const [setKeyValue, setSetKeyValue] = useState('');
  const [setKeyBaseUrl, setSetKeyBaseUrl] = useState('');
  const [setKeyModels, setSetKeyModels] = useState<string[]>([]);
  const [setKeyModelInput, setSetKeyModelInput] = useState('');
  const [deleteKeyConfirm, setDeleteKeyConfirm] = useState<string | null>(null);

  /* Providers list */
  const allProviderIds = providers.map((p) => p.providerId);
  const uniqueProviderIds = [...new Set(allProviderIds)];

  const providerById = (pid: string) => providers.find((p) => p.providerId === pid);
  const providerLabel = (pid: string) => providerById(pid)?.name ?? pid;

  /* ── Load client if not in store ─────────────────────── */
  useEffect(() => {
    fetchProviders();
    if (!client && id) {
      setLoading(true);
      dispatch(fetchClientThunk(id)).finally(() => setLoading(false));
    }
  }, [id, client, dispatch, fetchProviders]);

  /* ── Fetch provider keys ─────────────────────────────── */
  const fetchProviderKeys = useCallback(async (clientId: string) => {
    setProviderKeysLoading(true);
    try {
      const keys = await api.get<ClientProviderKeyListItem[]>(`/api/v1/admin/clients/${clientId}/provider-keys`);
      setProviderKeys(Array.isArray(keys) ? keys : []);
    } catch {
      setProviderKeys([]);
    } finally {
      setProviderKeysLoading(false);
    }
  }, []);

  useEffect(() => {
    if (client) {
      setEditName(client.name);
      setEditRoutes([...client.preferredProviders]);
      fetchProviderKeys(client.id);
    }
  }, [client?.id, fetchProviderKeys]); /* eslint-disable-line react-hooks/exhaustive-deps */

  /* ── Handlers ────────────────────────────────────────── */

  const handleSave = async () => {
    if (!client) return;
    const data: Record<string, any> = {};
    if (editName.trim() && editName.trim() !== client.name) {
      data.name = editName.trim();
    }
    if (JSON.stringify(editRoutes) !== JSON.stringify(client.preferredProviders)) {
      data.preferred_providers = editRoutes;
    }
    if (Object.keys(data).length === 0) {
      setEditing(false);
      return;
    }
    await dispatch(updateClient({ id: client.id, data })).unwrap();
    addToast('Client updated', 'success');
    setEditing(false);
  };

  const handleCancelEdit = () => {
    if (!client) return;
    setEditName(client.name);
    setEditRoutes([...client.preferredProviders]);
    setEditing(false);
  };

  const handleConfirmSave = async () => {
    setConfirmSave(false);
    await handleSave();
  };

  const handleRotate = async () => {
    if (!client) return;
    await dispatch(rotateKeys(client.id)).unwrap();
    setConfirmRotate(false);
    setSecretOpen('rotate');
  };

  const handleConfirmStatusChange = async () => {
    if (!client || !confirmStatusChange) return;
    await dispatch(updateClientStatus({ id: client.id, status: confirmStatusChange })).unwrap();
    addToast(`Client status updated to ${confirmStatusChange}`, 'info');
    setConfirmStatusChange(null);
  };

  const handleCloseSecret = () => {
    setSecretOpen(null);
    dispatch(clearSecrets());
  };

  /* ── Persistent credential storage (survives modal dismiss) ─ */
  const CRED_STORAGE_KEY = `ai-proxy-credentials-${client?.id}`;

  const saveCredentialsToStorage = (cid: string, secret: string, encKey: string, encSecret: string) => {
    try {
      localStorage.setItem(CRED_STORAGE_KEY, JSON.stringify({ cid, secret, encKey, encSecret }));
    } catch { /* storage full — silently ignore */ }
  };

  const getCredentialsFromStorage = (): { cid: string; secret: string; encKey: string; encSecret: string } | null => {
    try {
      const raw = localStorage.getItem(CRED_STORAGE_KEY);
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      if (parsed.cid && parsed.secret && parsed.encKey && parsed.encSecret) return parsed;
      return null;
    } catch {
      return null;
    }
  };

  /* ── Download handlers ───────────────────────────────── */

  const handleDownloadServerManagerFile = async () => {
    if (!client) return;

    // Try: localStorage → Redux store → API fetch for encryption creds
    const storedCreds = getCredentialsFromStorage();
    const reduxCreds =
      (rotateClientId && rotateSecret && rotateEncryptionKey && rotateEncryptionSecret
        ? { cid: rotateClientId, secret: rotateSecret, encKey: rotateEncryptionKey, encSecret: rotateEncryptionSecret }
        : null) ??
      (createClientId && createSecret && createEncryptionKey && createEncryptionSecret
        ? { cid: createClientId, secret: createSecret, encKey: createEncryptionKey, encSecret: createEncryptionSecret }
        : null);

    // We have all 4 — use as-is
    let cid: string;
    let secret: string | null;
    let encKey: string | null;
    let encSecret: string | null;

    if (storedCreds) {
      ({ cid, secret, encKey, encSecret } = storedCreds);
    } else if (reduxCreds) {
      ({ cid, secret, encKey, encSecret } = reduxCreds);
    } else {
      // Fallback: fetch encryption credentials from API (client secret is one-time)
      cid = client.clientId;
      secret = null;
      try {
        const resp = await api.get<{ encryption_key: string; encryption_secret: string }>(
          `/api/v1/admin/clients/${client.id}/credentials`,
        );
        encKey = resp.encryption_key;
        encSecret = resp.encryption_secret;
      } catch {
        encKey = null;
        encSecret = null;
      }
    }

    const lines: string[] = [
      '# AI Proxy — Server Manager Configuration',
      '# Store this file securely. It contains credentials that grant API access.',
      '',
      `# Generated: ${new Date().toISOString()}`,
      `# Client: ${client.name} (${client.clientId})`,
      `# Status: ${client.status}`,
      '',
      `CLIENT_ID=${cid}`,
    ];

    if (secret) {
      lines.push(`CLIENT_SECRET=${secret}`);
    } else {
      lines.push('# CLIENT_SECRET is only available at creation or rotation time.');
      lines.push('# To generate a new one, use the Rotate Keys action on this client.');
      lines.push('# CLIENT_SECRET=<generated-via-rotate>');
    }

    if (encKey) {
      lines.push(`ENCRYPTION_KEY=${encKey}`);
    } else {
      lines.push('# ENCRYPTION_KEY=<retrieve-from-admin>');
    }

    if (encSecret) {
      lines.push(`ENCRYPTION_SECRET=${encSecret}`);
    } else {
      lines.push('# ENCRYPTION_SECRET=<retrieve-from-admin>');
    }

    lines.push(
      '',
      '# Optional: proxy server URL (if different from default)',
      '# PROXY_SERVER_URL=http://localhost:8080',
      '',
      '# To use these credentials, set them as environment variables',
      '# or source this file in your shell:',
      '#   source ai-proxy-server-manager.env',
    );

    const content = lines.join('\n');
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `ai-proxy-server-manager-${client.clientId}-${new Date().toISOString().slice(0, 10)}.env`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const downloadCredentials = (mode: 'create' | 'rotate') => {
    const cid = mode === 'create' ? createClientId : rotateClientId;
    const secret = mode === 'create' ? createSecret : rotateSecret;
    const encKey = mode === 'create' ? createEncryptionKey : rotateEncryptionKey;
    const encSecret = mode === 'create' ? createEncryptionSecret : rotateEncryptionSecret;
    if (!cid || !secret || !encKey || !encSecret) return;

    const content = [
      '# AI Proxy Client Credentials',
      `# Generated: ${new Date().toISOString()}`,
      `# ${mode === 'create' ? 'Client created' : 'Keys rotated'} — store securely, never share`,
      '',
      `CLIENT_ID=${cid}`,
      `CLIENT_SECRET=${secret}`,
      `ENCRYPTION_KEY=${encKey}`,
      `ENCRYPTION_SECRET=${encSecret}`,
      '',
      '# To use these credentials, set them as environment variables',
      '# or pass them to your AI Proxy client configuration.',
    ].join('\n');

    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `ai-proxy-credentials-${new Date().toISOString().slice(0, 10)}.env`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  /* ── Provider key handlers ───────────────────────────── */

  const handleSetProviderKey = async () => {
    if (!client || !setKeyProvider || !setKeyValue.trim()) return;
    try {
      await api.put<SetClientProviderKeyResponse>(
        `/api/v1/admin/clients/${client.id}/provider-keys/${setKeyProvider}`,
        {
          api_key: setKeyValue.trim(),
          base_url: setKeyBaseUrl.trim() || undefined,
          models: setKeyModels, // empty = all models allowed
        },
      );
      addToast(`API key set for ${setKeyProvider}`, 'success');
      setSetKeyOpen(false);
      setSetKeyValue('');
      setSetKeyBaseUrl('');
      setSetKeyModels([]);
      setSetKeyModelInput('');
      fetchProviderKeys(client.id);
    } catch (err: any) {
      addToast(err?.message ?? 'Failed to set API key', 'error');
    }
  };

  const handleDeleteProviderKey = async () => {
    if (!client || !deleteKeyConfirm) return;
    try {
      await api.delete(`/api/v1/admin/clients/${client.id}/provider-keys/${deleteKeyConfirm}`);
      addToast(`API key removed for ${deleteKeyConfirm}`, 'success');
      setDeleteKeyConfirm(null);
      fetchProviderKeys(client.id);
    } catch (err: any) {
      addToast(err?.message ?? 'Failed to remove API key', 'error');
    }
  };

  const openSetKeyModal = (pid: string) => {
    setSetKeyProvider(pid);
    setSetKeyValue('');
    setSetKeyBaseUrl('');
    const existing = providerKeys.find((k) => k.provider === pid);
    setSetKeyModels(existing?.models ?? []);
    setSetKeyModelInput('');
    setSetKeyOpen(true);
  };

  const availableForNewRoute = uniqueProviderIds;

  /* ── Loading state ───────────────────────────────────── */
  if (loading) {
    return (
      <div className="max-w-6xl mx-auto px-6 py-8">
        <div className="flex items-center gap-2 text-muted-soft text-sm py-12 justify-center">
          <div className="w-5 h-5 rounded-full border-2 border-hairline border-t-primary animate-spin" />
          Loading client...
        </div>
      </div>
    );
  }

  if (!client) {
    return (
      <div className="max-w-6xl mx-auto px-6 py-8">
        <div className="text-center py-12">
          <p className="text-muted text-lg font-[500] mb-2">Client not found</p>
          <Button variant="secondary" onClick={() => navigate('/clients')}>
            Back to Clients
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Back link */}
      <button
        onClick={() => navigate('/clients')}
        className="
          inline-flex items-center gap-1.5
          text-muted hover:text-ink
          text-[13px] font-[500] leading-[1.23]
          transition-colors duration-150
          cursor-pointer bg-transparent border-none mb-6
        "
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M9 3L5 7l4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        Back to Clients
      </button>

      {/* Header */}
      <div className="flex items-start justify-between mb-8">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary font-[700] text-base">
            {client.name.charAt(0).toUpperCase()}
          </div>
          <div>
            <h1 className="text-ink text-[28px] font-[700] leading-[1.43] mb-0.5">{client.name}</h1>
            <div className="flex items-center gap-2">
              <Badge variant="status">{client.status}</Badge>
              <span className="text-muted-soft text-[13px] font-[400] leading-[1.23]">
                {client.clientId}
              </span>
              <CopyButton text={client.clientId} label="Client ID" />
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {editing ? (
            <>
              <Button variant="secondary" onClick={handleCancelEdit}>
                Cancel
              </Button>
              <Button variant="primary" onClick={() => {
                // Check if there are actual changes before showing confirm
                if (!client) return;
                const hasChanges = (
                  (editName.trim() && editName.trim() !== client.name) ||
                  JSON.stringify(editRoutes) !== JSON.stringify(client.preferredProviders)
                );
                if (hasChanges) {
                  setConfirmSave(true);
                } else {
                  setEditing(false);
                }
              }}>
                Save Changes
              </Button>
            </>
          ) : (
            <>
              <Button variant="tertiary-text" onClick={handleDownloadServerManagerFile}>
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-1.5">
                  <path d="M7 1v8M4 6l3 3 3-3M2 11v1a1 1 0 001 1h8a1 1 0 001-1v-1" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                Download Server File
              </Button>
              <Button variant="tertiary-text" onClick={() => setConfirmRotate(true)}>
                Rotate Keys
              </Button>
              <Button variant="secondary" onClick={() => setEditing(true)}>
                Edit
              </Button>
            </>
          )}
        </div>
      </div>

      {/* ── 2-Column Layout ───────────────────────────────── */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-8">

        {/* ── Left Column (2/5) ──────────────────────────── */}
        <div className="lg:col-span-2 flex flex-col gap-6">

          {/* Client Info Card */}
          <Card variant="default" className="p-5">
            <h2 className="text-ink text-[15px] font-[600] leading-[1.27] mb-4">Client Information</h2>

            <div className="flex flex-col gap-4">
              {/* Name */}
              <div>
                <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
                  Name
                </p>
                {editing ? (
                  <Input value={editName} onChange={(e) => setEditName(e.target.value)} />
                ) : (
                  <p className="text-ink text-sm font-[500] leading-[1.43]">{client.name}</p>
                )}
              </div>

              {/* Client ID */}
              <div>
                <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
                  Client ID
                </p>
                <div className="flex items-center gap-2 bg-surface-soft rounded-xs px-3 py-2.5 border border-hairline">
                  <code className="flex-1 text-ink text-sm font-[400] leading-[1.43] break-all select-all">
                    {client.clientId}
                  </code>
                  <CopyButton text={client.clientId} label="Client ID" />
                </div>
              </div>

              {/* Status */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
                    Status
                  </p>
                  <Badge variant="status">{client.status}</Badge>
                </div>
                <div>
                  <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
                    Created
                  </p>
                  <p className="text-ink text-sm font-[400] leading-[1.43]">
                    {new Date(client.createdAt).toLocaleDateString()}
                  </p>
                </div>
              </div>

              {/* Last Rotated */}
              <div>
                <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">
                  Last Key Rotation
                </p>
                <p className="text-ink text-sm font-[400] leading-[1.43]">
                  {client.lastRotatedAt
                    ? new Date(client.lastRotatedAt).toLocaleDateString()
                    : 'Never rotated'}
                </p>
              </div>
            </div>
          </Card>

          {/* Actions Card */}
          <Card variant="default" className="p-5">
            <h2 className="text-ink text-[15px] font-[600] leading-[1.27] mb-4">Actions</h2>
            <div className="flex flex-col gap-2.5">
              <Button
                variant="secondary"
                className="w-full justify-start"
                onClick={handleDownloadServerManagerFile}
              >
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-2">
                  <path d="M7 1v8M4 6l3 3 3-3M2 11v1a1 1 0 001 1h8a1 1 0 001-1v-1" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                Download Server Manager File
              </Button>

              <Button
                variant="secondary"
                className="w-full justify-start"
                onClick={() => setConfirmRotate(true)}
              >
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-2">
                  <path d="M1 7a6 6 0 0111.33-3M13 1v4h-4M13 7a6 6 0 01-11.33 3M1 13V9h4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                Rotate Keys
              </Button>

              <div className="border-t border-hairline my-1 pt-2.5">
                {client.status === 'active' ? (
                  <Button
                    variant="secondary"
                    className="w-full justify-start text-primary-error"
                    onClick={() => setConfirmStatusChange('suspended')}
                  >
                    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-2">
                      <path d="M5 3H3a1 1 0 00-1 1v6a1 1 0 001 1h2M9 3h2a1 1 0 011 1v6a1 1 0 01-1 1H9M5 10V4l4 3-4 3z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Suspend Client
                  </Button>
                ) : client.status === 'suspended' ? (
                  <Button
                    variant="primary"
                    className="w-full justify-start"
                    onClick={() => setConfirmStatusChange('active')}
                  >
                    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-2">
                      <path d="M4 3l7 4-7 4V3z" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Reactivate Client
                  </Button>
                ) : null}
              </div>
            </div>
          </Card>
        </div>

        {/* ── Right Column (3/5) ─────────────────────────── */}
        <div className="lg:col-span-3 flex flex-col gap-6">

          {/* Provider API Keys */}
          <Card variant="default" className="p-5">
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-ink text-[15px] font-[600] leading-[1.27]">Provider API Keys</h2>
              {!editing && uniqueProviderIds.length > 0 && (
                <Button variant="tertiary-text" onClick={() => {
                  const first = uniqueProviderIds.find(
                    (pid) => !providerKeys.find((k) => k.provider === pid && k.hasKey),
                  );
                  if (first) { openSetKeyModal(first); }
                }} className="text-[11px]">
                  + Add Key
                </Button>
              )}
            </div>
            <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-4">
              Set custom API keys for specific providers. Uses the global key if none is set.
            </p>

            {providerKeysLoading ? (
              <div className="flex items-center gap-2 text-muted-soft text-[13px] py-4">
                <div className="w-4 h-4 rounded-full border-2 border-hairline border-t-primary animate-spin" />
                Loading keys...
              </div>
            ) : (
              <ProviderKeyList
                uniqueProviderIds={uniqueProviderIds}
                providerKeys={providerKeys}
                providerLabel={providerLabel}
                editing={editing}
                onSetKey={openSetKeyModal}
                onDeleteKey={setDeleteKeyConfirm}
              />
            )}
          </Card>

          {/* Preferred Providers */}
          <Card variant="default" className="p-5">
            <h2 className="text-ink text-[15px] font-[600] leading-[1.27] mb-3">Preferred Routes</h2>

            {editing ? (
              <>
                <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-3">
                  Each route maps a model to a provider. Drag to reorder priority. Requests route to the first matching provider for the requested model.
                </p>

                {editRoutes.length > 0 ? (
                  <SortableList
                    items={editRoutes.map((_, i) => ({ id: `r-${i}` }))}
                    onChange={(items) => {
                      const reordered = items.map((item) => {
                        const idx = parseInt(item.id.replace('r-', ''), 10);
                        return editRoutes[idx];
                      });
                      setEditRoutes(reordered);
                    }}
                    onRemove={(id) => {
                      const idx = parseInt(id.replace('r-', ''), 10);
                      setEditRoutes((prev) => prev.filter((_, i) => i !== idx));
                    }}
                    renderItem={(item) => {
                      const idx = parseInt(item.id.replace('r-', ''), 10);
                      const r = editRoutes[idx];
                      return (
                        <div className="flex items-center gap-2 min-w-0">
                          <Badge variant="status">{r.provider}</Badge>
                          <span className="text-ink text-[13px] font-[500] leading-[1.23] truncate">
                            {r.model}
                          </span>
                        </div>
                      );
                    }}
                    className="mb-3"
                  />
                ) : (
                  <div className="mb-3 px-4 py-6 border border-dashed border-hairline rounded-xs bg-surface-soft/30 text-center">
                    <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">
                      No routes configured — will use default provider mapping
                    </p>
                  </div>
                )}

                {showAddRoute ? (
                  <div className="flex flex-col gap-2 p-3 rounded-xs border border-hairline bg-surface-soft/30">
                    <div className="flex gap-2">
                      <div className="flex-1">
                        <select
                          value={editNewProvider}
                          onChange={(e) => setEditNewProvider(e.target.value)}
                          className="
                            w-full px-3 py-2 rounded-xs
                            text-[13px] font-[400] leading-[1.23]
                            bg-surface-card text-ink
                            border border-hairline
                            focus:outline-none focus:border-ink
                            transition-colors duration-150
                            cursor-pointer
                          "
                        >
                          <option value="">Select provider…</option>
                          {uniqueProviderIds.map((pid) => (
                            <option key={pid} value={pid}>{providerLabel(pid)} ({pid})</option>
                          ))}
                        </select>
                      </div>
                      <div className="flex-[2]">
                        <Input
                          value={editNewModel}
                          onChange={(e) => setEditNewModel(e.target.value)}
                          placeholder="Model name, e.g. gpt-4o"
                        />
                      </div>
                    </div>
                    <div className="flex justify-end gap-2">
                      <Button variant="secondary" onClick={() => { setShowAddRoute(false); setEditNewProvider(''); setEditNewModel(''); }}>
                        Cancel
                      </Button>
                      <Button
                        variant="primary"
                        onClick={() => {
                          if (editNewProvider && editNewModel.trim()) {
                            setEditRoutes((prev) => [...prev, { provider: editNewProvider, model: editNewModel.trim() }]);
                            setEditNewProvider('');
                            setEditNewModel('');
                            setShowAddRoute(false);
                          }
                        }}
                        disabled={!editNewProvider || !editNewModel.trim()}
                      >
                        Add Route
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Button variant="tertiary-text" onClick={() => setShowAddRoute(true)} className="text-[12px]">
                    <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true" className="mr-1">
                      <path d="M5 1v8M1 5h8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                    </svg>
                    Add Route
                  </Button>
                )}
              </>
            ) : (
              <>
                {client.preferredProviders.length > 0 ? (
                  <div className="flex flex-col gap-2">
                    {client.preferredProviders.map((route, idx) => (
                      <div
                        key={idx}
                        className="flex items-center gap-3 px-4 py-3 rounded-xs border border-hairline bg-surface-card"
                      >
                        <span className="flex items-center justify-center w-6 h-6 rounded-full bg-surface-strong text-muted-soft text-[12px] font-[700] leading-none shrink-0">
                          {idx + 1}
                        </span>
                        <Badge variant="status">{route.provider}</Badge>
                        <span className="text-ink text-[13px] font-[500] leading-[1.23]">
                          {route.model}
                        </span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="px-4 py-8 border border-dashed border-hairline rounded-xs bg-surface-soft/30 text-center">
                    <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">
                      No routes configured — will use default provider mapping
                    </p>
                  </div>
                )}
              </>
            )}
          </Card>
        </div>
      </div>

      {/* ── Set Provider Key modal ─────────────────────────── */}
      <Modal
        open={setKeyOpen}
        onClose={() => { setSetKeyOpen(false); setSetKeyValue(''); setSetKeyBaseUrl(''); setSetKeyModels([]); setSetKeyModelInput(''); }}
        title={`Set API Key — ${providerLabel(setKeyProvider)}`}
      >
        <div className="flex flex-col gap-4">
          <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">
            Setting a custom API key for this provider will override the global key for this client.
            The key is encrypted at rest and will only be shown once.
          </p>
          <Input
            label="API Key"
            type="password"
            value={setKeyValue}
            onChange={(e) => setSetKeyValue(e.target.value)}
            placeholder="sk-..."
          />
          <Input
            label="Base URL (optional)"
            value={setKeyBaseUrl}
            onChange={(e) => setSetKeyBaseUrl(e.target.value)}
            placeholder="https://api.openai.com/v1"
          />

          {/* Models */}
          <div>
            <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-2">
              Allowed Models
            </p>
            <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-2">
              Leave empty to allow all models from the provider. Add specific models to restrict access.
            </p>
            <div className="flex gap-2 mb-2">
              <div className="flex-1">
                <Input
                  value={setKeyModelInput}
                  onChange={(e) => setSetKeyModelInput(e.target.value)}
                  placeholder="e.g. gpt-4o"
                  onKeyDown={(e: React.KeyboardEvent) => {
                    if (e.key === 'Enter' && setKeyModelInput.trim()) {
                      e.preventDefault();
                      setSetKeyModels((prev) => [...prev, setKeyModelInput.trim()]);
                      setSetKeyModelInput('');
                    }
                  }}
                />
              </div>
              <Button
                variant="secondary"
                onClick={() => {
                  if (setKeyModelInput.trim()) {
                    setSetKeyModels((prev) => [...prev, setKeyModelInput.trim()]);
                    setSetKeyModelInput('');
                  }
                }}
                disabled={!setKeyModelInput.trim()}
              >
                Add
              </Button>
            </div>
            {setKeyModels.length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {setKeyModels.map((m) => (
                  <span
                    key={m}
                    className="
                      inline-flex items-center gap-1 px-2 py-1 rounded-xs
                      text-[12px] font-[500] leading-[1.23]
                      bg-surface-strong text-ink
                    "
                  >
                    {m}
                    <button
                      type="button"
                      onClick={() => setSetKeyModels((prev) => prev.filter((x) => x !== m))}
                      className="text-muted hover:text-ink cursor-pointer bg-transparent border-none p-0 leading-none"
                      aria-label={`Remove ${m}`}
                    >
                      <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
                        <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
                      </svg>
                    </button>
                  </span>
                ))}
              </div>
            ) : (
              <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] italic">
                No models restricted — all models will be allowed
              </p>
            )}
          </div>

          <div className="flex justify-end gap-3 mt-2">
            <Button variant="secondary" onClick={() => { setSetKeyOpen(false); setSetKeyValue(''); setSetKeyBaseUrl(''); setSetKeyModels([]); setSetKeyModelInput(''); }}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleSetProviderKey} disabled={!setKeyValue.trim()}>
              Save Key
            </Button>
          </div>
        </div>
      </Modal>

      {/* ── Delete Provider Key confirm ────────────────────── */}
      <Modal
        open={deleteKeyConfirm !== null}
        onClose={() => setDeleteKeyConfirm(null)}
        title="Remove API Key"
        closeable={false}
      >
        <p className="text-ink text-sm font-[400] leading-[1.43] mb-6">
          Remove the custom API key for <strong>{providerLabel(deleteKeyConfirm ?? '')}</strong>?
          This client will fall back to the global provider key.
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setDeleteKeyConfirm(null)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleDeleteProviderKey}>
            Remove Key
          </Button>
        </div>
      </Modal>

      {/* ── Confirm status change modal ──────────────────────── */}
      <Modal
        open={confirmStatusChange !== null}
        onClose={() => setConfirmStatusChange(null)}
        title={confirmStatusChange === 'suspended' ? 'Suspend Client' : 'Reactivate Client'}
      >
        {confirmStatusChange === 'suspended' ? (
          <>
            <p className="text-ink text-sm font-[400] leading-[1.43] mb-1">
              Are you sure you want to suspend <strong>{client.name}</strong>?
            </p>
            <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-6">
              Suspended clients cannot make API requests until reactivated. Active long-running requests will be terminated.
            </p>
          </>
        ) : (
          <>
            <p className="text-ink text-sm font-[400] leading-[1.43] mb-1">
              Are you sure you want to reactivate <strong>{client.name}</strong>?
            </p>
            <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-6">
              The client will be able to make API requests again immediately.
            </p>
          </>
        )}
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setConfirmStatusChange(null)}>
            Cancel
          </Button>
          <Button
            variant={confirmStatusChange === 'suspended' ? 'primary' : 'primary'}
            onClick={handleConfirmStatusChange}
          >
            {confirmStatusChange === 'suspended' ? 'Suspend Client' : 'Reactivate Client'}
          </Button>
        </div>
      </Modal>

      {/* ── Confirm save changes modal ─────────────────────── */}
      <Modal
        open={confirmSave}
        onClose={() => setConfirmSave(false)}
        title="Save Changes"
      >
        <p className="text-ink text-sm font-[400] leading-[1.43] mb-1">
          Are you sure you want to save the changes to <strong>{client.name}</strong>?
        </p>
        <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-6">
          {editName.trim() !== client.name && (
            <span className="block">Name will change from &quot;{client.name}&quot; to &quot;{editName.trim()}&quot;</span>
          )}
          {JSON.stringify(editRoutes) !== JSON.stringify(client.preferredProviders) && (
            <span className="block">Preferred routes will be updated ({editRoutes.length} routes)</span>
          )}
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setConfirmSave(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleConfirmSave}>
            Save Changes
          </Button>
        </div>
      </Modal>

      {/* ── Confirm rotate modal ───────────────────────────── */}
      <Modal
        open={confirmRotate}
        onClose={() => setConfirmRotate(false)}
        title="Rotate API Keys"
      >
        <p className="text-ink text-sm font-[400] leading-[1.43] mb-6">
          This will revoke the current API keys and issue new ones. All active sessions using the old keys will be terminated. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setConfirmRotate(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={handleRotate}>
            Confirm Rotation
          </Button>
        </div>
      </Modal>

      {/* ── Secret reveal modal ────────────────────────────── */}
      <Modal
        open={secretOpen !== null}
        onClose={handleCloseSecret}
        title={secretOpen === 'create' ? 'Client Created' : 'Keys Rotated'}
      >
        <div className="flex flex-col gap-4">
          <div className="bg-amber-50 border border-amber-200 rounded-xs px-4 py-3">
            <p className="text-amber-800 text-sm font-[500] leading-[1.43]">
              {secretOpen === 'create'
                ? 'Store these credentials securely. They will not be shown again.'
                : 'The new credentials are shown below. The old credentials have been revoked.'}
            </p>
          </div>

          {(() => {
            const mode = secretOpen ?? 'create';
            const secret = mode === 'create' ? createSecret : rotateSecret;
            const encKey = mode === 'create' ? createEncryptionKey : rotateEncryptionKey;
            const encSecret = mode === 'create' ? createEncryptionSecret : rotateEncryptionSecret;
            const clientId = mode === 'create' ? createClientId : rotateClientId;

            // Persist to localStorage so Download Server Manager File always works
            if (clientId && secret && encKey && encSecret) {
              saveCredentialsToStorage(clientId, secret, encKey, encSecret);
            }

            return (
              <>
                <div>
                  <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">Client Secret</p>
                  <div className="flex items-center gap-2 bg-surface-soft rounded-xs px-3 py-2.5 border border-hairline">
                    <code className="flex-1 text-ink text-sm font-[400] leading-[1.43] break-all select-all">{secret}</code>
                    <CopyButton text={secret ?? ''} label="Client Secret" />
                  </div>
                </div>
                <div>
                  <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">Encryption Key</p>
                  <div className="flex items-center gap-2 bg-surface-soft rounded-xs px-3 py-2.5 border border-hairline">
                    <code className="flex-1 text-ink text-sm font-[400] leading-[1.43] break-all select-all">{encKey}</code>
                    <CopyButton text={encKey ?? ''} label="Encryption Key" />
                  </div>
                </div>
                <div>
                  <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-1">Encryption Secret</p>
                  <div className="flex items-center gap-2 bg-surface-soft rounded-xs px-3 py-2.5 border border-hairline">
                    <code className="flex-1 text-ink text-sm font-[400] leading-[1.43] break-all select-all">{encSecret}</code>
                    <CopyButton text={encSecret ?? ''} label="Encryption Secret" />
                  </div>
                </div>
                <div className="flex justify-end gap-3 mt-2">
                  <Button variant="secondary" onClick={() => downloadCredentials(mode)}>
                    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true" className="mr-1.5">
                      <path d="M7 1v8M4 6l3 3 3-3M2 11v1a1 1 0 001 1h8a1 1 0 001-1v-1" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Download .env File
                  </Button>
                  <Button variant="primary" onClick={handleCloseSecret}>
                    I've Saved the Credentials
                  </Button>
                </div>
              </>
            );
          })()}
        </div>
      </Modal>
    </div>
  );
};
