import type { ReactNode, HTMLAttributes } from 'react';

type CardVariant = 'default' | 'host' | 'reservation' | 'property';

interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: CardVariant;
  children: ReactNode;
  hoverable?: boolean;
  padding?: boolean;
}

const variantClasses: Record<CardVariant, string> = {
  default: 'bg-surface-card rounded-md',
  host: 'bg-surface-card rounded-md p-6',
  reservation:
    'bg-surface-card rounded-md border border-hairline shadow-card p-6',
  property: 'bg-surface-card rounded-md',
};

export const Card = ({
  variant = 'default',
  children,
  hoverable = false,
  padding = true,
  className = '',
  ...rest
}: CardProps) => {
  return (
    <div
      className={`
        ${variantClasses[variant]}
        ${padding && variant === 'default' ? 'p-4' : ''}
        ${hoverable ? 'transition-shadow duration-150 hover:shadow-card cursor-pointer' : ''}
        ${className}
      `}
      {...rest}
    >
      {children}
    </div>
  );
};
