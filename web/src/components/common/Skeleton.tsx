interface SkeletonProps {
  /** Height in Tailwind units (e.g. "h-4") */
  height?: string;
  /** Width in Tailwind units (e.g. "w-3/4") */
  width?: string;
  /** Number of skeleton lines to stack */
  lines?: number;
  className?: string;
}

export const Skeleton = ({
  height = 'h-4',
  width = 'w-full',
  lines = 1,
  className = '',
}: SkeletonProps) => {
  return (
    <div className={`flex flex-col gap-2 ${className}`} aria-hidden="true">
      {Array.from({ length: lines }).map((_, i) => (
        <div
          key={i}
          className={`${height} ${width} rounded-xs bg-surface-strong animate-pulse`}
          style={i > 0 ? { width: `${Math.max(40, 100 - i * 15)}%` } : undefined}
        />
      ))}
    </div>
  );
};
