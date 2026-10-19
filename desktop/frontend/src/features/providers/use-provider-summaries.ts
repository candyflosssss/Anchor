import { useEffect, useMemo, useRef, useState } from 'react';
import type { Provider, ProviderTransport } from './provider.model';
import type { Workspace } from './provider-features.model';
import { summarize, type ProviderSummary } from './provider-summary.model';
import { KeyedRefresh } from '../../lib/keyed-refresh';

type Entry = { revision: number; workspace: Workspace | null };
/** Reuse unchanged snapshots; only explicit refreshes reread every provider. */
export function useProviderSummaries(providers: Provider[], ready: boolean, transport: ProviderTransport, selected: Workspace | null, refreshVersion = 0): ProviderSummary[] {
  const [snapshot, setSnapshot] = useState<{ transport: ProviderTransport; entries: Record<string, Entry> }>({ transport, entries: {} });
  const session = useRef<KeyedRefresh<Provider> | null>(null);
  const current = useRef({ providers, refreshVersion }); current.current = { providers, refreshVersion };
  useEffect(() => {
    if (!ready || !transport.workspace) return;
    const queue = new KeyedRefresh<Provider>(async (p, signal) => {
      let workspace: Workspace | null = null;
      try { workspace = await transport.workspace!(p.id, signal); } catch { /* Show an unknown state, never fabricate health. */ }
      if (!signal.aborted) setSnapshot(old => ({ transport, entries: { ...(old.transport === transport ? old.entries : {}), [p.id]: { revision: p.revision, workspace } } }));
    });
    session.current = queue;
    queue.update(current.current.providers, current.current.refreshVersion);
    return () => { queue.dispose(); session.current = null; };
  }, [ready, transport]);
  useEffect(() => {
    session.current?.update(providers, refreshVersion);
    const ids = new Set(providers.map(p => p.id));
    setSnapshot(old => ({ ...old, entries: Object.fromEntries(Object.entries(old.entries).filter(([id]) => ids.has(id))) }));
  }, [providers, refreshVersion]);
  useEffect(() => {
    if (!selected || !providers.some(p => p.id === selected.provider_id && p.revision === selected.revision)) return;
    setSnapshot(old => ({ transport, entries: { ...(old.transport === transport ? old.entries : {}), [selected.provider_id]: { revision: selected.revision, workspace: selected } } }));
  }, [selected, providers, transport]);
  return useMemo(() => providers.map(p => {
    if (!transport.workspace) return summarize(p, null);
    if (selected && selected.provider_id === p.id && selected.revision === p.revision) return summarize(p, selected);
    const cached = snapshot.transport === transport ? snapshot.entries[p.id] : undefined;
    return summarize(p, cached?.revision === p.revision && (!cached.workspace || cached.workspace.revision === p.revision) ? cached.workspace : undefined);
  }), [providers, snapshot, selected, transport]);
}
