import { useState, useEffect, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';
import { Button, Input, Card } from '../components/common';

export const Login = () => {
  const { isAuthenticated, login, loading, error, clearError } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  useEffect(() => {
    if (isAuthenticated) {
      navigate('/', { replace: true });
    }
  }, [isAuthenticated, navigate]);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    clearError();
    login({ email, password });
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-surface-soft px-4">
      <Card variant="reservation" className="w-full" style={{ maxWidth: '32rem' }}>
        {/* Logo */}
        <div className="flex flex-col items-center mb-8">
          <div className="w-12 h-12 rounded-full bg-primary flex items-center justify-center mb-3">
            <svg width="24" height="24" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <path d="M8 2l1.5 3L13 5.5l-2.5 2.5.75 3.5L8 9.5l-3 1.5.75-3.5L3 5.5 6.5 5 8 2z" fill="white" />
            </svg>
          </div>
          <h1 className="text-ink text-[22px] font-[500] leading-[1.18] -tracking-[0.44px]">
            AI Proxy
          </h1>
          <p className="text-muted text-sm font-[400] leading-[1.43] mt-1">
            Sign in to your admin dashboard
          </p>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="admin@example.com"
            required
          />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter your password"
            required
          />

          {error && (
            <div className="bg-red-50 border border-red-200 rounded-xs px-4 py-3 text-primary-error text-sm font-[400] leading-[1.43]">
              {error}
            </div>
          )}

          <Button type="submit" variant="primary" loading={loading} className="w-full mt-2">
            Sign in
          </Button>
        </form>

        <p className="text-center text-muted-soft text-xs font-[400] leading-[1.23] mt-6">
          Sign in with your admin credentials
        </p>
      </Card>
    </div>
  );
};
