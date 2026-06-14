import type { ReactNode } from 'react';

interface Column<T> {
  key: string;
  header: string;
  render: (item: T) => ReactNode;
  className?: string;
}

interface TableProps<T> {
  columns: Column<T>[];
  data: T[];
  loading?: boolean;
  onRowClick?: (item: T) => void;
  emptyMessage?: string;
  keyExtractor: (item: T) => string;
}

export const Table = <T,>({
  columns,
  data,
  loading = false,
  onRowClick,
  emptyMessage = 'No data',
  keyExtractor,
}: TableProps<T>) => {
  if (loading) {
    return (
      <div className="w-full overflow-x-auto">
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  className="text-left text-muted text-sm font-[500] leading-[1.29] pb-3 border-b border-hairline"
                >
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <tr key={i}>
                {columns.map((col) => (
                  <td key={col.key} className="py-3 border-b border-hairline-soft">
                    <div className="h-4 bg-surface-strong rounded-xs animate-pulse w-3/4" />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="w-full text-center py-12 text-muted text-sm font-[400] leading-[1.43]">
        {emptyMessage}
      </div>
    );
  }

  return (
    <div className="w-full overflow-x-auto">
      <table className="w-full border-collapse">
        <thead>
          <tr>
            {columns.map((col) => (
              <th
                key={col.key}
                className={`text-left text-muted text-sm font-[500] leading-[1.29] pb-3 border-b border-hairline ${col.className ?? ''}`}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((item) => (
            <tr
              key={keyExtractor(item)}
              onClick={() => onRowClick?.(item)}
              className={`
                border-b border-hairline-soft
                ${onRowClick ? 'cursor-pointer hover:bg-surface-soft transition-colors duration-150' : ''}
              `}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  className={`py-3 text-ink text-sm font-[400] leading-[1.43] ${col.className ?? ''}`}
                >
                  {col.render(item)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};
