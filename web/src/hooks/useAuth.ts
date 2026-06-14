import { useCallback } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import type { AppDispatch } from '../store';
import {
  login,
  logout,
  clearError,
  selectIsAuthenticated,
  selectUser,
  selectAuthLoading,
  selectAuthError,
} from '../store/slices/authSlice';
import type { LoginRequest } from '../types';

export const useAuth = () => {
  const dispatch = useDispatch<AppDispatch>();

  const isAuthenticated = useSelector(selectIsAuthenticated);
  const user = useSelector(selectUser);
  const loading = useSelector(selectAuthLoading);
  const error = useSelector(selectAuthError);

  const handleLogin = useCallback(
    (credentials: LoginRequest) => dispatch(login(credentials)),
    [dispatch],
  );

  const handleLogout = useCallback(() => {
    dispatch(logout());
  }, [dispatch]);

  const handleClearError = useCallback(() => {
    dispatch(clearError());
  }, [dispatch]);

  return {
    isAuthenticated,
    user,
    loading,
    error,
    login: handleLogin,
    logout: handleLogout,
    clearError: handleClearError,
  };
};
