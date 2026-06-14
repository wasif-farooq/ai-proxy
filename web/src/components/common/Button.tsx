import type { ButtonHTMLAttributes, ReactNode } from 'react';

type ButtonVariant = 'primary' | 'secondary' | 'tertiary-text' | 'pill';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  children: ReactNode;
  loading?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  /* Rausch fill, white text, 8px radius, 14×24px padding, 48px height */
  primary:
    'bg-primary text-on-primary rounded-sm px-6 py-3.5 h-12 font-[500] text-base leading-[1.25] border-none hover:bg-primary-active disabled:bg-primary-disabled disabled:cursor-not-allowed transition-colors duration-150',
  /* White fill, ink text, 1px ink outline, 8px radius */
  secondary:
    'bg-canvas text-ink rounded-sm px-[23px] py-[13px] h-12 font-[500] text-base leading-[1.25] border border-ink hover:bg-surface-soft disabled:opacity-40 disabled:cursor-not-allowed transition-colors duration-150',
  /* Plain ink text, no surface, no border */
  'tertiary-text':
    'bg-transparent text-ink font-[500] text-base leading-[1.25] p-0 border-none underline-offset-2 hover:underline disabled:text-muted-soft disabled:cursor-not-allowed transition-colors duration-150',
  /* Pill-shaped rausch CTA — 9999px radius, 10×20px padding */
  pill:
    'bg-primary text-on-primary rounded-full px-5 py-2.5 font-[500] text-sm leading-[1.29] border-none hover:bg-primary-active disabled:bg-primary-disabled disabled:cursor-not-allowed transition-colors duration-150',
};

export const Button = ({
  variant = 'primary',
  children,
  loading = false,
  disabled,
  className = '',
  ...rest
}: ButtonProps) => {
  return (
    <button
      className={`inline-flex items-center justify-center gap-2 cursor-pointer select-none ${variantClasses[variant]} ${className}`}
      disabled={disabled || loading}
      {...rest}
    >
      {loading && (
        <svg
          className="animate-spin h-4 w-4"
          viewBox="0 0 24 24"
          fill="none"
          aria-hidden="true"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
          />
        </svg>
      )}
      {children}
    </button>
  );
};
