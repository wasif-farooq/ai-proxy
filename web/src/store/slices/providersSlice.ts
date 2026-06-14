import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import type { Provider } from '../../types';
import { api } from '../../services/api';

/* ─── State ──────────────────────────────────────────────── */

interface ProvidersState {
  items: Provider[];
  loading: boolean;
  error: string | null;
}

const initialState: ProvidersState = {
  items: [],
  loading: false,
  error: null,
};

/* ─── Map backend field names ────────────────────────────── */

function mapProvider(raw: any): Provider {
  return {
    id: raw.id,
    providerId: raw.provider_id ?? raw.providerId,
    name: raw.name,
    baseUrl: raw.base_url ?? raw.baseUrl ?? '',
    enabled: raw.enabled ?? true,
    models: raw.models ?? [],
    createdAt: raw.created_at ?? raw.createdAt,
    updatedAt: raw.updated_at ?? raw.updatedAt,
  };
}

/* ─── Thunks ─────────────────────────────────────────────── */

export const fetchProviders = createAsyncThunk('providers/fetchAll', async () => {
  const data: any[] = await api.get('/api/v1/admin/providers');
  return data.map(mapProvider);
});

export const createProvider = createAsyncThunk(
  'providers/create',
  async (input: { provider_id: string; name: string; api_key: string; base_url?: string; models?: string[] }) => {
    const result: any = await api.post('/api/v1/admin/providers', input);
    return mapProvider(result);
  },
);

export const updateProvider = createAsyncThunk(
  'providers/update',
  async ({ id, data }: { id: string; data: { name?: string; api_key?: string; base_url?: string; enabled?: boolean; models?: string[] } }) => {
    const result: any = await api.put(`/api/v1/admin/providers/${id}`, data);
    return mapProvider(result);
  },
);

export const deleteProvider = createAsyncThunk('providers/delete', async (id: string) => {
  await api.delete(`/api/v1/admin/providers/${id}`);
  return id;
});

/* ─── Slice ──────────────────────────────────────────────── */

const providersSlice = createSlice({
  name: 'providers',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchProviders.pending, (state) => { state.loading = true; state.error = null; })
      .addCase(fetchProviders.fulfilled, (state, action) => { state.loading = false; state.items = action.payload; })
      .addCase(fetchProviders.rejected, (state, action) => { state.loading = false; state.error = action.error.message ?? 'Failed to fetch providers'; })
      .addCase(createProvider.fulfilled, (state, action) => { state.items.push(action.payload); })
      .addCase(updateProvider.fulfilled, (state, action) => {
        const idx = state.items.findIndex((p) => p.id === action.payload.id);
        if (idx >= 0) state.items[idx] = action.payload;
      })
      .addCase(deleteProvider.fulfilled, (state, action) => {
        state.items = state.items.filter((p) => p.id !== action.payload);
      });
  },
});

/* ─── Selectors ──────────────────────────────────────────── */

export const selectProviders = (state: { providers: ProvidersState }) => state.providers.items;
export const selectProvidersLoading = (state: { providers: ProvidersState }) => state.providers.loading;
export const selectProvidersError = (state: { providers: ProvidersState }) => state.providers.error;

export default providersSlice.reducer;
