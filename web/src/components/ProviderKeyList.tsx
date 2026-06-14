import { Badge, Button } from './common';
import type { ClientProviderKeyListItem } from '../types';

interface ProviderKeyListProps {
  uniqueProviderIds: string[];
  providerKeys: ClientProviderKeyListItem[];
  providerLabel: (pid: string) => string;
  editing: boolean;
  onSetKey: (pid: string) => void;
  onDeleteKey: (pid: string | null) => void;
}

export const ProviderKeyList = ({
  uniqueProviderIds,
  providerKeys,
  providerLabel,
  editing,
  onSetKey,
  onDeleteKey,
}: ProviderKeyListProps) => {
  if (uniqueProviderIds.length === 0) {
    return (
      <div className="px-4 py-8 border border-dashed border-hairline rounded-xs bg-surface-soft/30 text-center">
        <p className="text-muted-soft text-[13px] font-[400] leading-[1.23]">
          No providers configured. Add providers in Settings first.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      {uniqueProviderIds.map((pid) => {
        const keyInfo = providerKeys.find((k) => k.provider === pid);
        const hasKey = keyInfo?.hasKey ?? false;

        return (
          <div
            key={pid}
            className="flex flex-col px-4 py-3 rounded-xs border border-hairline bg-surface-card hover:bg-surface-soft transition-colors duration-150"
          >
            <div className="flex items-center justify-between min-w-0">
              <div className="flex items-center gap-3 min-w-0">
                <div className={`w-2 h-2 rounded-full ${hasKey ? 'bg-success' : 'bg-muted-soft/40'}`} />
                <Badge variant="status">{pid}</Badge>
                <span className="text-ink text-[13px] font-[500] leading-[1.23]">
                  {providerLabel(pid)}
                </span>
                <span className={`text-[11px] font-[500] leading-[1.18] ${hasKey ? 'text-success' : 'text-muted-soft'}`}>
                  {hasKey ? 'Custom key set' : 'Using global key'}
                </span>
              </div>
              {!editing && (
                <div className="flex items-center gap-1.5 shrink-0">
                  <Button variant="tertiary-text" onClick={() => onSetKey(pid)} className="text-[11px]">
                    {hasKey ? 'Update' : 'Set Key'}
                  </Button>
                  {hasKey && (
                    <Button variant="tertiary-text" onClick={() => onDeleteKey(pid)} className="text-[11px] text-primary-error">
                      <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
                        <path d="M2 3h8M4.5 3V2a1 1 0 011-1h1a1 1 0 011 1v1M3 3v6.5A1.5 1.5 0 004.5 11h3A1.5 1.5 0 009 9.5V3" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round" />
                      </svg>
                      Remove
                    </Button>
                  )}
                </div>
              )}
            </div>
            {/* Allowed models */}
            {hasKey && keyInfo?.models && keyInfo.models.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-2 ml-5">
                {keyInfo.models.map((m) => (
                  <span
                    key={m}
                    className="text-muted text-[11px] font-[500] bg-surface-soft rounded-xs px-2 py-0.5"
                  >
                    {m}
                  </span>
                ))}
              </div>
            )}
            {hasKey && (!keyInfo?.models || keyInfo.models.length === 0) && (
              <p className="text-muted-soft text-[11px] font-[400] ml-5 mt-1">
                All models allowed
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
};
