import type { ReactNode } from 'react';

type BadgeVariant = 'default' | 'guest-favorite' | 'new' | 'status';

interface BadgeProps {
  variant?: BadgeVariant;
  children: ReactNode;
}

/**
 * Guest-favorite badge — white rounded pill at 11px / 600 over a photo.
 * New tag — tiny uppercase pill at 8px / 700 with tracking.
 * Status badge — for client status (active / suspended / revoked).
 */
const variantClasses: Record<BadgeVariant, string> = {
  default:
    'bg-canvas text-ink rounded-full px-2.5 py-1 text-[11px] font-[600] leading-[1.18] shadow-card',
  'guest-favorite':
    'bg-canvas text-ink rounded-full px-2.5 py-1 text-[11px] font-[600] leading-[1.18] shadow-card',
  new:
    'bg-canvas text-ink rounded-full px-1.5 py-0.5 text-[8px] font-[700] leading-[1.25] tracking-[0.32px] uppercase',
  status:
    'rounded-full px-2.5 py-0.5 text-[11px] font-[600] leading-[1.18]',
};

const statusColors: Record<string, string> = {
  active: 'bg-green-50 text-green-700',
  suspended: 'bg-amber-50 text-amber-700',
  revoked: 'bg-red-50 text-red-700',
};

export const Badge = ({ variant = 'default', children }: BadgeProps) => {
  const colorClass =
    variant === 'status' && typeof children === 'string'
      ? statusColors[children.toLowerCase()] ?? 'bg-surface-soft text-muted'
      : '';

  return (
    <span className={`inline-flex items-center ${variantClasses[variant]} ${colorClass}`}>
      {variant === 'guest-favorite' && (
        <svg className="w-3 h-3 mr-1" viewBox="0 0 12 12" fill="none" aria-hidden="true">
          <path
            d="M6 1l1.5 3 3.25.5-2.5 2.5.75 3.5L6 8.5l-3 1.5.75-3.5L1.25 4.5 4.5 4 6 1z"
            fill="currentColor"
          />
        </svg>
      )}
      {children}
    </span>
  );
};
