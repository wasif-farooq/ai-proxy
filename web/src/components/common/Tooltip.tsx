import { useState } from 'react';

interface TooltipProps {
  text: string;
  children: React.ReactNode;
}

export const Tooltip = ({ text, children }: TooltipProps) => {
  const [visible, setVisible] = useState(false);

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      onFocus={() => setVisible(true)}
      onBlur={() => setVisible(false)}
    >
      {children}
      {visible && (
        <span
          className="
            absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5
            px-2 py-1 rounded-xs
            text-[11px] font-[500] leading-[1.18] whitespace-nowrap
            bg-ink text-surface-card
            pointer-events-none z-50
            shadow-lg
          "
        >
          {text}
        </span>
      )}
    </div>
  );
};
