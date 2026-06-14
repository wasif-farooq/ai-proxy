import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import type { AuthUser, LoginRequest } from '../../types';
import { api, setAuthToken, getAuthToken } from '../../services/api';

interface AuthState {
  user: AuthUser | null;
  token: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
}

function loadInitialState(): AuthState {
  const token = getAuthToken();
  const stored = localStorage.getItem('auth_user');
  let user: AuthUser | null = null;
  if (token && stored) {
    try {
      user = JSON.parse(stored) as AuthUser;
    } catch {
      localStorage.removeItem('auth_user');
    }
  }
  return {
    user,
    token,
    isAuthenticated: !!token && !!user,
    loading: false,
    error: null,
  };
}

const initialState: AuthState = loadInitialState();

export const login = createAsyncThunk(
  'auth/login',
  async (credentials: LoginRequest) => {
    const result = await api.login(credentials.email, credentials.password);
    // result: { token, admin_id, email, name, role, expires_at }
    setAuthToken(result.token);
    const user: AuthUser = {
      id: result.admin_id,
      email: result.email,
      name: result.name,
      role: result.role as AuthUser['role'],
    };
    localStorage.setItem('auth_user', JSON.stringify(user));
    return { user, token: result.token };
  },
);

export const fetchMe = createAsyncThunk(
  'auth/fetchMe',
  async () => {
    const data = await api.getMe();
    const user: AuthUser = {
      id: data.admin_id,
      email: data.email,
      name: data.name,
      role: data.role as AuthUser['role'],
    };
    localStorage.setItem('auth_user', JSON.stringify(user));
    return user;
  },
);

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    logout(state) {
      state.user = null;
      state.token = null;
      state.isAuthenticated = false;
      state.error = null;
      setAuthToken(null);
      localStorage.removeItem('auth_user');
    },
    clearError(state) {
      state.error = null;
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.pending, (state) => {
        state.loading = true;
        state.error = null;
      })
      .addCase(login.fulfilled, (state, action) => {
        state.loading = false;
        state.user = action.payload.user;
        state.token = action.payload.token;
        state.isAuthenticated = true;
      })
      .addCase(login.rejected, (state, action) => {
        state.loading = false;
        state.error = action.error.message ?? 'Login failed';
      })
      .addCase(fetchMe.fulfilled, (state, action) => {
        state.user = action.payload;
      });
  },
});

export const { logout, clearError } = authSlice.actions;

/* ─── Selectors ──────────────────────────────────────────── */

export const selectIsAuthenticated = (state: { auth: AuthState }) => state.auth.isAuthenticated;
export const selectUser = (state: { auth: AuthState }) => state.auth.user;
export const selectAuthLoading = (state: { auth: AuthState }) => state.auth.loading;
export const selectAuthError = (state: { auth: AuthState }) => state.auth.error;

export default authSlice.reducer;
