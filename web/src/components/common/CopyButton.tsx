import { useState, useCallback } from 'react';

interface CopyButtonProps {
  text: string;
  label?: string;
}

export const CopyButton = ({ text, label }: CopyButtonProps) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for older browsers
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [text]);

  return (
    <button
      onClick={handleCopy}
      className={`
        inline-flex items-center justify-center gap-1.5 shrink-0
        px-2 py-1 rounded-xs
        text-[11px] font-[600] leading-[1.18] uppercase tracking-wider
        border-none cursor-pointer select-none
        transition-all duration-150
        ${copied
          ? 'bg-green-100 text-green-700'
          : 'bg-surface-strong text-muted hover:bg-hairline hover:text-ink'
        }
      `}
      title={`Copy ${label ?? text}`}
      aria-label={`Copy ${label ?? text}`}
    >
      {copied ? (
        <>
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
            <path d="M2.5 6L5 8.5L9.5 3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          Copied
        </>
      ) : (
        <>
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
            <rect x="3.5" y="3.5" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.2" />
            <path d="M8.5 3V2.5C8.5 1.67 7.83 1 7 1H2.5C1.67 1 1 1.67 1 2.5V7C1 7.83 1.67 8.5 2.5 8.5H3" stroke="currentColor" strokeWidth="1.2" />
          </svg>
          Copy
        </>
      )}
    </button>
  );
};
