import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import type { Client, CreateClientRequest, ClientStatus, PreferredRoute } from '../../types';
import { api } from '../../services/api';

/* ─── State ──────────────────────────────────────────────── */

interface ClientsState {
  items: Client[];
  selectedId: string | null;
  loading: boolean;
  error: string | null;
  rotateSecret: string | null;
  createSecret: string | null;
  createClientId: string | null;
  createEncryptionKey: string | null;
  createEncryptionSecret: string | null;
  rotateClientId: string | null;
  rotateEncryptionKey: string | null;
  rotateEncryptionSecret: string | null;
}

const initialState: ClientsState = {
  items: [],
  selectedId: null,
  loading: false,
  error: null,
  rotateSecret: null,
  createSecret: null,
  createClientId: null,
  createEncryptionKey: null,
  createEncryptionSecret: null,
  rotateClientId: null,
  rotateEncryptionKey: null,
  rotateEncryptionSecret: null,
};

/* ─── Map backend field names ────────────────────────────── */

function mapClient(raw: any): Client {
  const providers = raw.preferred_providers ?? raw.preferredProviders ?? [];
  // Handle both new format [{provider, model}] and old format ["provider_id"]
  const preferredProviders: PreferredRoute[] = Array.isArray(providers)
    ? providers.map((p: any) =>
        typeof p === 'string'
          ? { provider: p, model: '' }
          : { provider: p.provider ?? '', model: p.model ?? '' },
      )
    : [];
  return {
    id: raw.id,
    clientId: raw.client_id ?? raw.clientId,
    name: raw.name,
    status: raw.status,
    preferredProviders,
    createdAt: raw.created_at ?? raw.createdAt,
    updatedAt: raw.updated_at ?? raw.updatedAt,
    lastRotatedAt: raw.last_rotated_at ?? raw.lastRotatedAt ?? null,
  };
}

/* ─── Thunks ─────────────────────────────────────────────── */

export const fetchClients = createAsyncThunk('clients/fetchAll', async () => {
  const data: any[] = await api.get('/api/v1/admin/clients');
  return data.map(mapClient);
});

export const createClient = createAsyncThunk(
  'clients/create',
  async (data: CreateClientRequest) => {
    const result: any = await api.post('/api/v1/admin/clients', {
      name: data.name,
      preferred_providers: data.preferredProviders,
    });
    // result: { client: {...}, client_secret: "sk-...", encryption_key: "...", encryption_secret: "...", secret_warning: "..." }
    return {
      client: mapClient(result.client),
      clientId: result.client.client_id,
      secret: result.client_secret,
      encryptionKey: result.encryption_key,
      encryptionSecret: result.encryption_secret,
    };
  },
);

export const fetchClient = createAsyncThunk('clients/fetchOne', async (id: string) => {
  const data: any = await api.get(`/api/v1/admin/clients/${id}`);
  return mapClient(data);
});

export const updateClient = createAsyncThunk(
  'clients/update',
  async ({ id, data }: { id: string; data: { name?: string; status?: ClientStatus; preferred_providers?: string[] } }) => {
    const result: any = await api.put(`/api/v1/admin/clients/${id}`, data);
    return mapClient(result);
  },
);

export const deleteClient = createAsyncThunk('clients/delete', async (id: string) => {
  await api.delete(`/api/v1/admin/clients/${id}`);
  return id;
});

export const rotateKeys = createAsyncThunk(
  'clients/rotateKeys',
  async (clientId: string) => {
    const result: any = await api.post(`/api/v1/admin/clients/${clientId}/rotate`);
    return {
      client: mapClient(result.client),
      clientId: result.client.client_id,
      secret: result.client_secret,
      encryptionKey: result.encryption_key,
      encryptionSecret: result.encryption_secret,
    };
  },
);

export const updateClientStatus = createAsyncThunk(
  'clients/updateStatus',
  async ({ id, status }: { id: string; status: ClientStatus }) => {
    const result: any = await api.put(`/api/v1/admin/clients/${id}`, { status });
    return mapClient(result);
  },
);

/* ─── Slice ──────────────────────────────────────────────── */

const clientsSlice = createSlice({
  name: 'clients',
  initialState,
  reducers: {
    selectClient(state, action) {
      state.selectedId = action.payload;
    },
    clearSelection(state) {
      state.selectedId = null;
      state.createSecret = null;
      state.rotateSecret = null;
      state.createClientId = null;
      state.createEncryptionKey = null;
      state.createEncryptionSecret = null;
      state.rotateClientId = null;
      state.rotateEncryptionKey = null;
      state.rotateEncryptionSecret = null;
    },
    clearSecrets(state) {
      state.createSecret = null;
      state.rotateSecret = null;
      state.createClientId = null;
      state.createEncryptionKey = null;
      state.createEncryptionSecret = null;
      state.rotateClientId = null;
      state.rotateEncryptionKey = null;
      state.rotateEncryptionSecret = null;
    },
  },
  extraReducers: (builder) => {
    builder
      /* fetch all */
      .addCase(fetchClients.pending, (state) => { state.loading = true; state.error = null; })
      .addCase(fetchClients.fulfilled, (state, action) => { state.loading = false; state.items = action.payload; })
      .addCase(fetchClients.rejected, (state, action) => { state.loading = false; state.error = action.error.message ?? 'Failed to fetch clients'; })
      /* create */
      .addCase(createClient.pending, (state) => { state.error = null; })
      .addCase(createClient.fulfilled, (state, action) => {
        state.items.unshift(action.payload.client);
        state.createSecret = action.payload.secret;
        state.createClientId = action.payload.clientId;
        state.createEncryptionKey = action.payload.encryptionKey;
        state.createEncryptionSecret = action.payload.encryptionSecret;
      })
      .addCase(createClient.rejected, (state, action) => { state.error = action.error.message ?? 'Failed to create client'; })
      /* update */
      .addCase(updateClient.fulfilled, (state, action) => {
        const idx = state.items.findIndex((c) => c.id === action.payload.id);
        if (idx >= 0) state.items[idx] = action.payload;
      })
      /* delete */
      .addCase(deleteClient.fulfilled, (state, action) => {
        state.items = state.items.filter((c) => c.id !== action.payload);
      })
      /* rotate */
      .addCase(rotateKeys.fulfilled, (state, action) => {
        const idx = state.items.findIndex((c) => c.id === action.payload.client.id);
        if (idx >= 0) state.items[idx] = action.payload.client;
        state.rotateSecret = action.payload.secret;
        state.rotateClientId = action.payload.clientId;
        state.rotateEncryptionKey = action.payload.encryptionKey;
        state.rotateEncryptionSecret = action.payload.encryptionSecret;
      })
      /* update status */
      .addCase(updateClientStatus.fulfilled, (state, action) => {
        const idx = state.items.findIndex((c) => c.id === action.payload.id);
        if (idx >= 0) state.items[idx] = action.payload;
      });
  },
});

export const { selectClient, clearSelection, clearSecrets } = clientsSlice.actions;

/* ─── Selectors ──────────────────────────────────────────── */

export const selectClients = (state: { clients: ClientsState }) => state.clients.items;
export const selectClientsLoading = (state: { clients: ClientsState }) => state.clients.loading;
export const selectClientsError = (state: { clients: ClientsState }) => state.clients.error;
export const selectSelectedClient = (state: { clients: ClientsState }) =>
  state.clients.items.find((c) => c.id === state.clients.selectedId) ?? null;
export const selectCreateSecret = (state: { clients: ClientsState }) => state.clients.createSecret;
export const selectRotateSecret = (state: { clients: ClientsState }) => state.clients.rotateSecret;
export const selectCreateClientId = (state: { clients: ClientsState }) => state.clients.createClientId;
export const selectCreateEncryptionKey = (state: { clients: ClientsState }) => state.clients.createEncryptionKey;
export const selectCreateEncryptionSecret = (state: { clients: ClientsState }) => state.clients.createEncryptionSecret;
export const selectRotateClientId = (state: { clients: ClientsState }) => state.clients.rotateClientId;
export const selectRotateEncryptionKey = (state: { clients: ClientsState }) => state.clients.rotateEncryptionKey;
export const selectRotateEncryptionSecret = (state: { clients: ClientsState }) => state.clients.rotateEncryptionSecret;

export default clientsSlice.reducer;
