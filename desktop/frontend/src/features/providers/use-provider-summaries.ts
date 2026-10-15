import { useEffect, useMemo, useRef, useState } from 'react';
import type { Provider, ProviderTransport } from './provider.model';
import type { Workspace } from './provider-features.model';
import { summarize, type ProviderSummary } from './provider-summary.model';

/**
 * Reads every provider workspace in parallel so list/overview can show health, keys and models
 * without opening each detail. Read-only; old entries stay visible while a new round loads.
 */
export function useProviderSummaries(providers: Provider[], ready: boolean, transport: ProviderTransport, selected: Workspace | null): ProviderSummary[] {
  const [entries, setEntries] = useState<Record<string, Workspace | null>>({});
  const generation = useRef(0);
  const hasReader = !!transport.workspace;
  useEffect(() => {
    if (!ready || !transport.workspace) return;
    const reader = transport.workspace.bind(transport);
    const abort = new AbortController(); const epoch = ++generation.current;
    for (const p of providers) {
      reader(p.id, abort.signal)
        .then(w => { if (!abort.signal.aborted && epoch === generation.current) setEntries(old => ({ ...old, [p.id]: w })); })
        .catch(() => { if (!abort.signal.aborted && epoch === generation.current) setEntries(old => ({ ...old, [p.id]: null })); });
    }
    return () => abort.abort();
  }, [providers, ready, transport]);
  return useMemo(() => providers.map(p => {
    if (!hasReader) return summarize(p, null);
    if (selected && selected.provider_id === p.id && selected.revision === p.revision) return summarize(p, selected);
    const cached = entries[p.id];
    return summarize(p, cached === null || cached?.revision === p.revision ? cached : undefined);
  }), [providers, entries, selected, hasReader]);
}
