import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useClients } from '../hooks/useClients';
import { useProviders } from '../hooks/useProviders';
import { Button, Table, Modal, Input, Badge, Card, CopyButton, SortableList, Tooltip } from '../components/common';
import type { Client, PreferredRoute } from '../types';

/* ─── Clients Page ─────────────────────────────────────── */

export const Clients = () => {
  const navigate = useNavigate();
  const {
    clients,
    loading,
    fetchClients,
    createClient,
    rotateKeys,
    createSecret,
    rotateSecret,
    createClientId,
    createEncryptionKey,
    createEncryptionSecret,
    rotateClientId,
    rotateEncryptionKey,
    rotateEncryptionSecret,
    clearSecrets,
  } = useClients();
  const { providers, fetchProviders } = useProviders();

  /* ── Modal states ──────────────────────────────────── */
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmRotate, setConfirmRotate] = useState<string | null>(null);
  const [secretOpen, setSecretOpen] = useState<'create' | 'rotate' | null>(null);

  /* ── Create form ───────────────────────────────────── */
  const [newName, setNewName] = useState('');
  const [newRoutes, setNewRoutes] = useState<PreferredRoute[]>([]);
  const [newRouteProvider, setNewRouteProvider] = useState('');
  const [newRouteModel, setNewRouteModel] = useState('');
  const [showAddForm, setShowAddForm] = useState(false);

  /* ── Provider helpers ──────────────────────────────── */
  const allProviderIds = providers.map((p) => p.providerId);
  const uniqueProviderIds = [...new Set(allProviderIds)];

  const providerById = (id: string) => providers.find((p) => p.providerId === id);
  const providerLabel = (id: string) => providerById(id)?.name ?? id;

  useEffect(() => {
    fetchClients();
    fetchProviders();
  }, [fetchClients, fetchProviders]);

  /* ── Handlers ──────────────────────────────────────── */

  const handleCreate = async () => {
    if (!newName.trim()) return;
    await createClient({
      name: newName.trim(),
      preferredProviders: newRoutes.length > 0 ? newRoutes : undefined,
    });
    setNewName('');
    setNewRoutes([]);
    setCreateOpen(false);
    setSecretOpen('create');
  };

  const handleRotate = async (clientId: string) => {
    await rotateKeys(clientId);
    setConfirmRotate(null);
    setSecretOpen('rotate');
  };

  const handleCloseSecret = () => {
    setSecretOpen(null);
    clearSecrets();
  };

  const handleDownloadCredentials = () => {
    if (!secretOpen) return;
    const clientId = secretOpen === 'create' ? createClientId : rotateClientId;
    const secret = secretOpen === 'create' ? createSecret : rotateSecret;
    const encKey = secretOpen === 'create' ? createEncryptionKey : rotateEncryptionKey;
    const encSecret = secretOpen === 'create' ? createEncryptionSecret : rotateEncryptionSecret;
    if (!clientId || !secret || !encKey || !encSecret) return;

    const content = [
      '# AI Proxy Client Credentials',
      `# Generated: ${new Date().toISOString()}`,
      `# ${secretOpen === 'create' ? 'Client created' : 'Keys rotated'} — store securely, never share`,
      '',
      `CLIENT_ID=${clientId}`,
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
    const routeLabel = (r: PreferredRoute) => `${providerLabel(r.provider)} — ${r.model}`;
    console.log(routeLabel); // suppress unused warning
    a.href = url;
    a.download = `ai-proxy-credentials-${new Date().toISOString().slice(0, 10)}.env`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const addRoute = () => {
    if (newRouteProvider && newRouteModel.trim()) {
      setNewRoutes((prev) => [...prev, { provider: newRouteProvider, model: newRouteModel.trim() }]);
      setNewRouteProvider('');
      setNewRouteModel('');
      setShowAddForm(false);
    }
  };

  const removeRoute = (index: number) => {
    setNewRoutes((prev) => prev.filter((_, i) => i !== index));
  };

  const routeLabel = (r: PreferredRoute) => `${providerLabel(r.provider)} — ${r.model}`;

  /* ── Table columns ─────────────────────────────────── */
  const columns = [
    {
      key: 'name',
      header: 'Name',
      render: (c: Client) => (
        <span className="font-[500]">{c.name}</span>
      ),
    },
    {
      key: 'clientId',
      header: 'Client ID',
      render: (c: Client) => (
        <div className="flex items-center gap-1.5">
          <code className="text-muted text-[13px]">{c.clientId}</code>
          <CopyButton text={c.clientId} label="Client ID" />
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (c: Client) => <Badge variant="status">{c.status}</Badge>,
    },
    {
      key: 'routes',
      header: 'Routes',
      render: (c: Client) => (
        <div className="flex flex-wrap gap-1">
          {c.preferredProviders.length > 0 ? (
            c.preferredProviders.map((r, i) => (
              <span key={i} className="inline-flex items-center gap-1 text-[11px] font-[500] bg-surface-soft rounded-xs px-2 py-0.5">
                <Badge variant="status">{r.provider}</Badge>
                <span className="text-muted">{r.model}</span>
              </span>
            ))
          ) : (
            <span className="text-muted-soft text-[13px]">—</span>
          )}
        </div>
      ),
    },
    {
      key: 'lastRotated',
      header: 'Last Rotated',
      render: (c: Client) => (
        <span className="text-muted text-[13px]">
          {c.lastRotatedAt ? new Date(c.lastRotatedAt).toLocaleDateString() : 'Never'}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      className: 'text-right',
      render: (c: Client) => (
        <div className="flex items-center justify-end gap-0.5">
          <Tooltip text="Rotate keys">
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); setConfirmRotate(c.id); }}
              className="
                p-2 rounded-xs
                text-muted-soft hover:text-ink hover:bg-surface-soft
                transition-all duration-150
                cursor-pointer bg-transparent border-none
              "
              aria-label="Rotate keys"
            >
              <svg width="15" height="15" viewBox="0 0 15 15" fill="none" aria-hidden="true">
                <path d="M1.5 7.5a6 6 0 0111.33-3M13.5 1.5v4h-4M13.5 7.5a6 6 0 01-11.33 3M1.5 13.5v-4h4"
                  stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </button>
          </Tooltip>
          <Tooltip text="View details">
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); navigate(`/clients/${c.id}`); }}
              className="
                p-2 rounded-xs
                text-muted-soft hover:text-ink hover:bg-surface-soft
                transition-all duration-150
                cursor-pointer bg-transparent border-none
              "
              aria-label="View details"
            >
              <svg width="15" height="15" viewBox="0 0 15 15" fill="none" aria-hidden="true">
                <path d="M7.5 3.5c-3.5 0-6 3.5-6 4s2.5 4 6 4 6-3.5 6-4-2.5-4-6-4z"
                  stroke="currentColor" strokeWidth="1.3" />
                <circle cx="7.5" cy="7.5" r="1.5" stroke="currentColor" strokeWidth="1.3" />
              </svg>
            </button>
          </Tooltip>
        </div>
      ),
    },
  ];

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-ink text-[28px] font-[700] leading-[1.43] mb-1">Clients</h1>
          <p className="text-muted text-sm font-[400] leading-[1.43]">
            Manage API clients and their credentials
          </p>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}>
          Create Client
        </Button>
      </div>

      {/* Table */}
      <Card variant="default" className="p-0">
        <Table
          columns={columns}
          data={clients}
          loading={loading}
          onRowClick={(c) => navigate(`/clients/${c.id}`)}
          emptyMessage="No clients found. Create your first client to get started."
          keyExtractor={(c) => c.id}
        />
      </Card>

      {/* ── Create Client Modal ──────────────────────────── */}
      <Modal open={createOpen} onClose={() => { setCreateOpen(false); setNewRoutes([]); }} title="Create Client">
        <div className="flex flex-col gap-4">
          <Input
            label="Client Name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="e.g. Production App"
          />

          <div>
            <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-2">
              Preferred Routes
            </p>
            <p className="text-muted-soft text-[13px] font-[400] leading-[1.23] mb-2">
              Drag to reorder priority. Each route maps a model to a provider. The first matching route wins.
            </p>

            {newRoutes.length > 0 ? (
              <SortableList
                items={newRoutes.map((r, i) => ({ id: `r-${i}` }))}
                onChange={(items) => {
                  const reordered = items.map((item) => {
                    const idx = parseInt(item.id.replace('r-', ''), 10);
                    return newRoutes[idx];
                  });
                  setNewRoutes(reordered);
                }}
                onRemove={(id) => {
                  const idx = parseInt(id.replace('r-', ''), 10);
                  removeRoute(idx);
                }}
                renderItem={(item) => {
                  const idx = parseInt(item.id.replace('r-', ''), 10);
                  const r = newRoutes[idx];
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

            {/* Add route form */}
            {showAddForm ? (
              <div className="flex flex-col gap-2 p-3 rounded-xs border border-hairline bg-surface-soft/30">
                <div className="flex gap-2">
                  <div className="flex-1">
                    <select
                      value={newRouteProvider}
                      onChange={(e) => setNewRouteProvider(e.target.value)}
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
                      value={newRouteModel}
                      onChange={(e) => setNewRouteModel(e.target.value)}
                      placeholder="Model name, e.g. gpt-4o"
                    />
                  </div>
                </div>
                <div className="flex justify-end gap-2">
                  <Button variant="secondary" onClick={() => { setShowAddForm(false); setNewRouteProvider(''); setNewRouteModel(''); }}>
                    Cancel
                  </Button>
                  <Button variant="primary" onClick={addRoute} disabled={!newRouteProvider || !newRouteModel.trim()}>
                    Add Route
                  </Button>
                </div>
              </div>
            ) : (
              <Button variant="tertiary-text" onClick={() => setShowAddForm(true)} className="text-[12px]">
                <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true" className="mr-1">
                  <path d="M5 1v8M1 5h8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                </svg>
                Add Route
              </Button>
            )}
          </div>

          <div className="flex justify-end gap-3 mt-2">
            <Button variant="secondary" onClick={() => { setCreateOpen(false); setNewRoutes([]); }}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleCreate} disabled={!newName.trim()}>
              Create
            </Button>
          </div>
        </div>
      </Modal>

      {/* Confirm rotate modal */}
      <Modal
        open={confirmRotate !== null}
        onClose={() => setConfirmRotate(null)}
        title="Rotate API Keys"
      >
        <p className="text-ink text-sm font-[400] leading-[1.43] mb-6">
          This will revoke the current API keys and issue new ones. All active sessions using the old keys will be terminated. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setConfirmRotate(null)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => confirmRotate && handleRotate(confirmRotate)}
          >
            Confirm Rotation
          </Button>
        </div>
      </Modal>

      {/* Secret reveal modal */}
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
                  <Button variant="secondary" onClick={handleDownloadCredentials}>
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
