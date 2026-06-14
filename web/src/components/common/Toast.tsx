import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';

type ToastSeverity = 'success' | 'error' | 'info';

interface Toast {
  id: string;
  message: string;
  severity: ToastSeverity;
}

interface ToastContextValue {
  toasts: Toast[];
  addToast: (message: string, severity?: ToastSeverity) => void;
  removeToast: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export const useToast = () => {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within a ToastProvider');
  return ctx;
};

export const ToastProvider = ({ children }: { children: ReactNode }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((message: string, severity: ToastSeverity = 'info') => {
    const id = crypto.randomUUID();
    setToasts((prev) => [...prev, { id, message, severity }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  return (
    <ToastContext.Provider value={{ toasts, addToast, removeToast }}>
      {children}
      {/* Toast container */}
      <div className="fixed bottom-6 right-6 z-50 flex flex-col gap-2" style={{ maxWidth: '24rem' }}>
        {toasts.map((toast) => {
          const bgMap: Record<ToastSeverity, string> = {
            success: 'bg-green-600',
            error: 'bg-primary-error',
            info: 'bg-ink',
          };
          return (
            <div
              key={toast.id}
              className={`
                ${bgMap[toast.severity]}
                text-on-primary text-sm font-[400] leading-[1.43]
                rounded-sm px-4 py-3
                shadow-modal
                flex items-center gap-3
                animate-[slideIn_0.2s_ease-out]
              `}
            >
              <span className="flex-1">{toast.message}</span>
              <button
                onClick={() => removeToast(toast.id)}
                className="bg-transparent border-none text-on-primary/70 hover:text-on-primary cursor-pointer p-0 text-base leading-none"
                aria-label="Dismiss"
              >
                &times;
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
};
