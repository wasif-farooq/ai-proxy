import { useEffect, useState, useCallback } from 'react';
import { Card, Button, Input, Modal, Table, Badge, SortableList } from '../components/common';
import { useProviders } from '../hooks/useProviders';
import type { Provider } from '../types';

export const Settings = () => {
  const { providers, loading, fetchProviders, createProvider, updateProvider, deleteProvider } = useProviders();

  const [createOpen, setCreateOpen] = useState(false);
  const [editProvider, setEditProvider] = useState<Provider | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  // Create form state
  const [newProviderId, setNewProviderId] = useState('openai');
  const [newName, setNewName] = useState('');
  const [newApiKey, setNewApiKey] = useState('');
  const [newBaseUrl, setNewBaseUrl] = useState('');
  const [newModels, setNewModels] = useState<string[]>([]);
  const [newModelInput, setNewModelInput] = useState('');

  // Edit form state
  const [editName, setEditName] = useState('');
  const [editApiKey, setEditApiKey] = useState('');
  const [editBaseUrl, setEditBaseUrl] = useState('');
  const [editModels, setEditModels] = useState<string[]>([]);
  const [editModelInput, setEditModelInput] = useState('');
  const [editEnabled, setEditEnabled] = useState(true);

  useEffect(() => {
    fetchProviders();
  }, [fetchProviders]);

  const handleCreate = async () => {
    if (!newName.trim() || !newApiKey.trim()) return;
    await createProvider({
      provider_id: newProviderId,
      name: newName.trim(),
      api_key: newApiKey.trim(),
      base_url: newBaseUrl.trim() || undefined,
      models: newModels,
    });
    setCreateOpen(false);
    resetCreateForm();
  };

  const resetCreateForm = () => {
    setNewProviderId('openai');
    setNewName('');
    setNewApiKey('');
    setNewBaseUrl('');
    setNewModels([]);
    setNewModelInput('');
  };

  const openEdit = (p: Provider) => {
    setEditProvider(p);
    setEditName(p.name);
    setEditApiKey(''); // Don't pre-fill API key
    setEditBaseUrl(p.baseUrl);
    setEditModels([...p.models]);
    setEditModelInput('');
    setEditEnabled(p.enabled);
  };

  const handleUpdate = async () => {
    if (!editProvider) return;
    const data: Record<string, any> = { name: editName };
    if (editApiKey.trim()) data.api_key = editApiKey.trim();
    if (editBaseUrl.trim()) data.base_url = editBaseUrl.trim();
    data.models = editModels;
    data.enabled = editEnabled;
    await updateProvider(editProvider.id, data);
    setEditProvider(null);
  };

  const handleDelete = async () => {
    if (!deleteConfirm) return;
    await deleteProvider(deleteConfirm);
    setDeleteConfirm(null);
  };

  const columns = [
    {
      key: 'name',
      header: 'Name',
      render: (p: Provider) => <span className="font-[500]">{p.name}</span>,
    },
    {
      key: 'providerId',
      header: 'Provider',
      render: (p: Provider) => <Badge variant="status">{p.providerId}</Badge>,
    },
    {
      key: 'enabled',
      header: 'Status',
      render: (p: Provider) => (
        <Badge variant="status">{p.enabled ? 'Enabled' : 'Disabled'}</Badge>
      ),
    },
    {
      key: 'models',
      header: 'Models',
      render: (p: Provider) => (
        <div className="flex flex-wrap gap-1">
          {p.models.slice(0, 3).map((m) => (
            <span key={m} className="text-muted text-[11px] font-[500] bg-surface-soft rounded-xs px-2 py-0.5">
              {m}
            </span>
          ))}
          {p.models.length > 3 && (
            <span className="text-muted-soft text-[11px]">+{p.models.length - 3}</span>
          )}
        </div>
      ),
    },
    {
      key: 'baseUrl',
      header: 'Base URL',
      render: (p: Provider) => (
        <code className="text-muted text-[13px]">{p.baseUrl}</code>
      ),
    },
    {
      key: 'actions',
      header: '',
      className: 'text-right',
      render: (p: Provider) => (
        <div className="flex items-center justify-end gap-2">
          <Button variant="tertiary-text" onClick={() => openEdit(p)} className="text-[13px]">Edit</Button>
          <Button variant="tertiary-text" onClick={() => setDeleteConfirm(p.id)} className="text-[13px] text-primary-error">Delete</Button>
        </div>
      ),
    },
  ];

  return (
    <div className="max-w-5xl mx-auto px-6 py-8">
      <h1 className="text-ink text-[28px] font-[700] leading-[1.43] mb-1">Settings</h1>
      <p className="text-muted text-sm font-[400] leading-[1.43] mb-8">
        Manage your AI Proxy configuration
      </p>

      {/* Providers Section */}
      <Card variant="host" className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-ink text-[20px] font-[600] leading-[1.2] -tracking-[0.18px]">
              AI Providers
            </h2>
            <p className="text-muted text-[13px] font-[400] leading-[1.23] mt-1">
              Manage upstream AI provider connections
            </p>
          </div>
          <Button variant="primary" onClick={() => setCreateOpen(true)}>
            Add Provider
          </Button>
        </div>
        <Table
          columns={columns}
          data={providers}
          loading={loading}
          emptyMessage="No providers configured. Add an OpenAI, Anthropic, or other provider to get started."
          keyExtractor={(p) => p.id}
        />
      </Card>

      {/* Profile Section */}
      <Card variant="host" className="mb-6">
        <h2 className="text-ink text-[20px] font-[600] leading-[1.2] -tracking-[0.18px] mb-4">
          Profile
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
          <Input label="Name" defaultValue="Admin User" />
          <Input label="Email" defaultValue="admin@example.com" type="email" />
        </div>
        <Button variant="secondary">Save Changes</Button>
      </Card>

      {/* Security Section */}
      <Card variant="host" className="mb-6">
        <h2 className="text-ink text-[20px] font-[600] leading-[1.2] -tracking-[0.18px] mb-4">
          Security
        </h2>
        <div className="flex flex-col gap-4">
          <div>
            <p className="text-ink text-sm font-[500] leading-[1.29]">Change Password</p>
            <p className="text-muted text-[13px] font-[400] leading-[1.23] mb-3">
              Update your admin account password
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
              <Input label="Current Password" type="password" />
              <Input label="New Password" type="password" />
            </div>
            <Button variant="secondary">Update Password</Button>
          </div>
        </div>
      </Card>

      {/* Create Provider Modal */}
      <Modal open={createOpen} onClose={() => { setCreateOpen(false); resetCreateForm(); }} title="Add Provider">
        <div className="flex flex-col gap-4">
          <Input
            label="Provider ID"
            value={newProviderId}
            onChange={(e) => setNewProviderId(e.target.value)}
            placeholder="openai, anthropic, google, azure, ollama, deepseek, custom"
          />
          <Input
            label="Display Name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="e.g. OpenAI Production"
          />
          <Input
            label="API Key"
            type="password"
            value={newApiKey}
            onChange={(e) => setNewApiKey(e.target.value)}
            placeholder="sk-..."
          />
          <Input
            label="Base URL (optional)"
            value={newBaseUrl}
            onChange={(e) => setNewBaseUrl(e.target.value)}
            placeholder="https://api.openai.com/v1"
          />
          {/* Models as sortable list */}
          <div>
            <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-2">
              Models
            </p>
            <div className="flex gap-2 mb-2">
              <div className="flex-1">
                <Input
                  value={newModelInput}
                  onChange={(e) => setNewModelInput(e.target.value)}
                  placeholder="e.g. gpt-4o"
                  onKeyDown={(e: React.KeyboardEvent) => {
                    if (e.key === 'Enter' && newModelInput.trim()) {
                      e.preventDefault();
                      setNewModels((prev) => [...prev, newModelInput.trim()]);
                      setNewModelInput('');
                    }
                  }}
                />
              </div>
              <Button
                variant="secondary"
                onClick={() => {
                  if (newModelInput.trim()) {
                    setNewModels((prev) => [...prev, newModelInput.trim()]);
                    setNewModelInput('');
                  }
                }}
                disabled={!newModelInput.trim()}
              >
                Add
              </Button>
            </div>
            {newModels.length > 0 ? (
              <SortableList
                items={newModels.map((m) => ({ id: m }))}
                onChange={(items) => setNewModels(items.map((i) => i.id))}
                onRemove={(id) => setNewModels((prev) => prev.filter((m) => m !== id))}
                renderItem={(item) => (
                  <span className="text-ink text-[13px] font-[500] leading-[1.23]">{item.id}</span>
                )}
              />
            ) : (
              <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">No models added</p>
            )}
          </div>
          <div className="flex justify-end gap-3 mt-2">
            <Button variant="secondary" onClick={() => { setCreateOpen(false); resetCreateForm(); }}>Cancel</Button>
            <Button variant="primary" onClick={handleCreate} disabled={!newName.trim() || !newApiKey.trim()}>Save</Button>
          </div>
        </div>
      </Modal>

      {/* Edit Provider Modal */}
      <Modal open={!!editProvider} onClose={() => setEditProvider(null)} title={`Edit ${editProvider?.name ?? ''}`}>
        <div className="flex flex-col gap-4">
          <Input label="Display Name" value={editName} onChange={(e) => setEditName(e.target.value)} />
          <Input label="New API Key (leave blank to keep current)" type="password" value={editApiKey} onChange={(e) => setEditApiKey(e.target.value)} />
          <Input label="Base URL" value={editBaseUrl} onChange={(e) => setEditBaseUrl(e.target.value)} />
          {/* Models as sortable list */}
          <div>
            <p className="text-muted text-[11px] font-[600] leading-[1.18] uppercase tracking-wider mb-2">
              Models
            </p>
            <div className="flex gap-2 mb-2">
              <div className="flex-1">
                <Input
                  value={editModelInput}
                  onChange={(e) => setEditModelInput(e.target.value)}
                  placeholder="e.g. gpt-4o"
                  onKeyDown={(e: React.KeyboardEvent) => {
                    if (e.key === 'Enter' && editModelInput.trim()) {
                      e.preventDefault();
                      setEditModels((prev) => [...prev, editModelInput.trim()]);
                      setEditModelInput('');
                    }
                  }}
                />
              </div>
              <Button
                variant="secondary"
                onClick={() => {
                  if (editModelInput.trim()) {
                    setEditModels((prev) => [...prev, editModelInput.trim()]);
                    setEditModelInput('');
                  }
                }}
                disabled={!editModelInput.trim()}
              >
                Add
              </Button>
            </div>
            {editModels.length > 0 ? (
              <SortableList
                items={editModels.map((m) => ({ id: m }))}
                onChange={(items) => setEditModels(items.map((i) => i.id))}
                onRemove={(id) => setEditModels((prev) => prev.filter((m) => m !== id))}
                renderItem={(item) => (
                  <span className="text-ink text-[13px] font-[500] leading-[1.23]">{item.id}</span>
                )}
              />
            ) : (
              <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">No models added</p>
            )}
          </div>
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={editEnabled}
              onChange={(e) => setEditEnabled(e.target.checked)}
              className="w-4 h-4 rounded-xs border-hairline text-primary focus:ring-ink cursor-pointer"
            />
            <span className="text-ink text-sm font-[500] leading-[1.29]">Enabled</span>
          </label>
          <div className="flex justify-end gap-3 mt-2">
            <Button variant="secondary" onClick={() => setEditProvider(null)}>Cancel</Button>
            <Button variant="primary" onClick={handleUpdate}>Save</Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirm Modal */}
      <Modal open={!!deleteConfirm} onClose={() => setDeleteConfirm(null)} title="Delete Provider">
        <p className="text-ink text-sm font-[400] leading-[1.43] mb-6">
          Are you sure you want to delete this provider? Requests that route to this provider will fail. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setDeleteConfirm(null)}>Cancel</Button>
          <Button variant="primary" onClick={handleDelete}>Delete</Button>
        </div>
      </Modal>
    </div>
  );
};
