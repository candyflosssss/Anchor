import { useEffect, useState } from 'react';
import type { Suggestion } from '../../desktop/contract.generated';
import { logService } from './log.service';

export type SuggestionField = 'search' | 'provider' | 'model' | 'key' | 'http' | 'code' | 'stage' | 'agent' | 'binding' | 'project' | 'session';
type Result = { key: string; items: Suggestion[]; hasMore: boolean; failed: boolean };

// Only the focused control loads candidates. No page cursor or active filters
// participate, and cleanup prevents old tab/range/input responses being shown.
export function useLogSuggestions(kind: 'requests' | 'system', window: string, field: SuggestionField, prefix: string, enabled: boolean, revision: number) {
  const key = JSON.stringify([kind, window, field, prefix, revision]);
  const [result, setResult] = useState<Result | null>(null);
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      const now = Date.now();
      void logService.suggestions({ kind, field, prefix, since_ms: now - Number(window) * 60000, before_ms: now, limit: 20 })
        .then(v => { if (!cancelled) setResult({ key, items: v.items, hasMore: v.has_more, failed: false }); })
        .catch(() => { if (!cancelled) setResult({ key, items: [], hasMore: false, failed: true }); });
    }, 180);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [kind, window, field, prefix, enabled, revision, key]);
  const current = enabled && result?.key === key ? result : null;
  return { items: current?.items ?? [], hasMore: current?.hasMore ?? false, failed: current?.failed ?? false, loading: enabled && !current };
}
