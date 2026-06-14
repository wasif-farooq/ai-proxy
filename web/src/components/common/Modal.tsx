import type { ReactNode } from 'react';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  children: ReactNode;
  /** When false, clicking the scrim overlay does not close the modal and the X button is hidden. Default true. */
  closeable?: boolean;
}

export const Modal = ({ open, onClose, title, children, closeable = true }: ModalProps) => {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Scrim */}
      <div
        className="absolute inset-0 bg-scrim/50"
        onClick={closeable ? onClose : undefined}
        aria-hidden="true"
      />
      {/* Sheet */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="
          relative z-10 w-full
          bg-surface-card rounded-md
          shadow-modal
          p-6 mx-4
          max-h-[90vh] overflow-y-auto
        "
        style={{ maxWidth: '32rem' }}
      >
        {title && (
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-ink text-[20px] font-[600] leading-[1.2] -tracking-[0.18px]">
              {title}
            </h2>
            {closeable && (
              <button
                onClick={onClose}
                className="
                  flex items-center justify-center
                  w-8 h-8 rounded-full
                  bg-surface-strong text-ink
                  hover:bg-hairline
                  transition-colors duration-150
                  cursor-pointer border-none
                "
                aria-label="Close"
              >
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path d="M12 4L4 12M4 4l8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                </svg>
              </button>
            )}
          </div>
        )}
        {children}
      </div>
    </div>
  );
};
