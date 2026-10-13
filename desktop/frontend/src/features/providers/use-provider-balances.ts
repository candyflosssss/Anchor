import { useEffect, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { checkedAccountObservation, type AccountObservation } from './provider-account.model';

export interface ProviderBalance { observation?: AccountObservation; error?: string }

/** Default-key observations are bounded to two queries and owned by one list snapshot. */
export function useProviderBalances(providers: Provider[], active: boolean, transport: ProviderTransport): Record<string, ProviderBalance> {
  const [snapshot, setSnapshot] = useState<{ providers: Provider[]; transport: ProviderTransport; entries: Record<string, ProviderBalance> } | null>(null);
  useEffect(() => {
    if (!active) return;
    const abort = new AbortController();
    let next = 0;
    setSnapshot({ providers, transport, entries: {} });
    async function worker() {
      while (!abort.signal.aborted && next < providers.length) {
        const p = providers[next++];
        const q = { provider_id: p.id, expected_revision: p.revision, operation: 'balance-summary', key_id: '', remote_key_id: 0 };
        let entry: ProviderBalance;
        try {
          if (!transport.allowance) throw new ProviderProblem('read_error');
          entry = { observation: checkedAccountObservation(await transport.allowance(q, abort.signal), q) };
        } catch (e) {
          entry = { error: problemText[e instanceof ProviderProblem ? e.code : 'read_error'] };
        }
        if (abort.signal.aborted) return;
        setSnapshot(old => old?.providers === providers && old.transport === transport ? { ...old, entries: { ...old.entries, [p.id]: entry } } : old);
      }
    }
    void worker(); void worker();
    return () => abort.abort();
  }, [providers, active, transport]);
  return active && snapshot?.providers === providers && snapshot.transport === transport ? snapshot.entries : {};
}
