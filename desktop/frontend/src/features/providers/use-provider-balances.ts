import { useCallback, useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { accountFailureText, checkedAccountObservation } from './provider-account.model';
import { ProviderBalanceCache, type ProviderBalance } from './provider-balance.model';

type Snapshot = { providers: Provider[]; transport: ProviderTransport; cache: ProviderBalanceCache; entries: Record<string, ProviderBalance> };
type RefreshSession = { providers: Provider[]; enqueue: (provider: Provider) => void };

/** Show cached observations immediately; automatic and manual refreshes share two workers. */
export function useProviderBalances(providers: Provider[], active: boolean, transport: ProviderTransport, cache: ProviderBalanceCache) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const session = useRef<RefreshSession | null>(null);
  useEffect(() => {
    if (!active) return;
    cache.prune(providers);
    const abort = new AbortController();
    const queued: Provider[] = [];
    const pending = new Set<string>();
    let running = 0;
    setSnapshot({ providers, transport, cache, entries: cache.snapshot(providers) });

    function publish(provider: Provider, value: ProviderBalance) {
      setSnapshot(old => old?.providers === providers && old.transport === transport && old.cache === cache
        ? { ...old, entries: { ...old.entries, [provider.id]: value } } : old);
    }
    async function query(provider: Provider) {
      const q = { provider_id: provider.id, expected_revision: provider.revision, operation: 'balance-summary', key_id: '', remote_key_id: 0 };
      const previous = cache.read(provider);
      let value: ProviderBalance;
      try {
        if (!transport.allowance) throw new ProviderProblem('read_error');
        const observation = checkedAccountObservation(await transport.allowance(q, abort.signal), q);
        value = observation.failure
          ? { observation: previous?.observation ?? observation, failure: observation.failure,
              error: accountFailureText[observation.failure] ?? '查询失败，请稍后刷新。' }
          : { observation };
      } catch (e) {
        value = { observation: previous?.observation, error: problemText[e instanceof ProviderProblem ? e.code : 'read_error'] };
      }
      if (abort.signal.aborted) return;
      cache.write(provider, value);
      publish(provider, value);
      pending.delete(provider.id);
      running--;
      drain();
    }
    function drain() {
      while (!abort.signal.aborted && running < 2 && queued.length) {
        running++;
        void query(queued.shift()!);
      }
    }
    function enqueue(provider: Provider) {
      if (abort.signal.aborted || pending.has(provider.id)) return;
      pending.add(provider.id);
      publish(provider, { ...cache.read(provider), refreshing: true });
      queued.push(provider);
      drain();
    }
    const current = { providers, enqueue };
    session.current = current;
    for (const provider of providers) enqueue(provider);
    return () => {
      abort.abort();
      if (session.current === current) session.current = null;
    };
  }, [providers, active, transport, cache]);

  const refresh = useCallback((id: string) => {
    const current = session.current;
    if (!current) return;
    const provider = current.providers.find(provider => provider.id === id);
    if (provider) current.enqueue(provider);
  }, []);
  const entries = active && snapshot?.providers === providers && snapshot.transport === transport && snapshot.cache === cache
    ? snapshot.entries : cache.snapshot(providers);
  return { entries, refresh };
}
