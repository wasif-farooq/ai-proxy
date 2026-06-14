import type { InputHTMLAttributes } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

export const Input = ({ label, error, className = '', id, ...rest }: InputProps) => {
  const inputId = id ?? label?.toLowerCase().replace(/\s+/g, '-');

  return (
    <div className="flex flex-col gap-1.5">
      {label && (
        <label
          htmlFor={inputId}
          className="text-muted text-sm font-[500] leading-[1.29]"
        >
          {label}
        </label>
      )}
      <input
        id={inputId}
        className={`
          w-full h-14 rounded-sm px-3.5 py-3.5
          bg-canvas text-ink text-base font-[400] leading-[1.5]
          border border-hairline
          placeholder:text-muted
          focus:outline-none focus:border-ink focus:border-2
          disabled:bg-surface-soft disabled:cursor-not-allowed disabled:text-muted
          transition-colors duration-150
          ${error ? 'border-primary-error text-primary-error' : ''}
          ${className}
        `}
        {...rest}
      />
      {error && (
        <span className="text-primary-error text-sm font-[400] leading-[1.43] mt-0.5">
          {error}
        </span>
      )}
    </div>
  );
};
