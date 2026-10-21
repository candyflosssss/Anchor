import { useCallback, useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { accountFailureText, checkedAccountObservation } from './provider-account.model';
import { ProviderBalanceCache, type ProviderBalance } from './provider-balance.model';
import { KeyedRefresh, type RefreshTarget } from '../../lib/keyed-refresh';

type Snapshot = { transport: ProviderTransport; cache: ProviderBalanceCache; entries: Record<string, { revision: number; value: ProviderBalance }> };

/** Cached observations remain visible; changes and manual refreshes share two workers. */
export function useProviderBalances(providers: Provider[], active: boolean, transport: ProviderTransport, cache: ProviderBalanceCache, refreshVersion = 0) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const session = useRef<KeyedRefresh<RefreshTarget> | null>(null);
  const current = useRef({ providers, refreshVersion }); current.current = { providers, refreshVersion };
  useEffect(() => {
    if (!active) return;
    function publish(provider: Provider, value: ProviderBalance) {
      setSnapshot(old => ({ transport, cache, entries: { ...(old?.transport === transport && old.cache === cache ? old.entries : {}), [provider.id]: { revision: provider.revision, value } } }));
    }
    const queue = new KeyedRefresh<RefreshTarget>(async (target, signal) => {
      const provider = current.current.providers.find(p => p.id === target.id);
      if (!provider || cache.queryRevision(provider) !== target.revision) return;
      const q = { provider_id: provider.id, expected_revision: provider.revision, operation: 'balance-summary', key_id: '', remote_key_id: 0 };
      const previous = cache.read(provider);
      let value: ProviderBalance;
      try {
        if (!transport.allowance) throw new ProviderProblem('read_error');
        const observation = checkedAccountObservation(await transport.allowance(q, signal), q);
        value = observation.failure
          ? { observation: previous?.observation ?? observation, failure: observation.failure, error: accountFailureText[observation.failure] ?? '查询失败，请稍后刷新。' }
          : { observation };
      } catch (e) {
        value = { observation: previous?.observation, error: problemText[e instanceof ProviderProblem ? e.code : 'read_error'] };
      }
      if (signal.aborted) return;
      const latest = current.current.providers.find(p => p.id === target.id);
      if (!latest || cache.queryRevision(latest) !== target.revision) return;
      cache.write(latest, value); publish(latest, value);
    }, 2, target => {
      const provider = current.current.providers.find(p => p.id === target.id);
      if (provider) publish(provider, { ...cache.read(provider), refreshing: true });
    });
    session.current = queue;
    queue.update(current.current.providers.map(p => ({ id: p.id, revision: cache.queryRevision(p) })), current.current.refreshVersion);
    return () => { queue.dispose(); session.current = null; setSnapshot(null); };
  }, [active, transport, cache]);
  useEffect(() => {
    if (!active) return; // An initial empty list is not an authoritative deletion.
    cache.prune(providers);
    session.current?.update(providers.map(p => ({ id: p.id, revision: cache.queryRevision(p) })), refreshVersion);
    const ids = new Set(providers.map(p => p.id));
    setSnapshot(old => old && ({ ...old, entries: Object.fromEntries(Object.entries(old.entries).filter(([id]) => ids.has(id))) }));
  }, [providers, refreshVersion, cache, active]);
  const refresh = useCallback((id: string) => session.current?.refresh(id), []);
  const entries = cache.snapshot(providers);
  if (active && snapshot?.transport === transport && snapshot.cache === cache) for (const provider of providers) {
    const entry = snapshot.entries[provider.id];
    if (entry?.revision === provider.revision) entries[provider.id] = entry.value;
  }
  return { entries, refresh };
}
