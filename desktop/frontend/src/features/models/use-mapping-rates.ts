import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from '../providers/provider.model';
import type { AccountRateObservation } from '../providers/provider-account.model';

/** One explicit read per provider, serialized and bound to the current configuration. */
export function useMappingRates(providers: Provider[], active: boolean, transport: ProviderTransport) {
  const [values, setValues] = useState<Record<string, AccountRateObservation>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState('');
  const lock = useRef(false), epoch = useRef(0), abortRef = useRef<AbortController | null>(null);
  const signature = JSON.stringify(providers.map(p => [p.id, p.revision]));
  useEffect(() => {
    const at = ++epoch.current;
    abortRef.current?.abort(); lock.current = false;
    setValues({}); setErrors({}); setBusy(false); setPending('');
    return () => { epoch.current = at + 1; abortRef.current?.abort(); lock.current = false; };
  }, [signature, active, transport]);
  async function refresh(ids: string[]) {
    if (!active || lock.current) return;
    const targets = [...new Set(ids)].map(id => providers.find(p => p.id === id)).filter((p): p is Provider => !!p);
    if (!targets.length) return;
    lock.current = true; setBusy(true);
    const at = epoch.current, abort = new AbortController(); abortRef.current = abort;
    try {
      for (const p of targets) {
        if (abort.signal.aborted || at !== epoch.current) break;
        setPending(p.id);
        setValues(old => { const next = { ...old }; delete next[p.id]; return next; });
        setErrors(old => { const next = { ...old }; delete next[p.id]; return next; });
        try {
          if (!transport.rate) throw new ProviderProblem('read_error');
          const value = await transport.rate({ provider_id: p.id, expected_revision: p.revision, operation: 'effective-rate', key_id: '', remote_key_id: 0 }, abort.signal);
          if (abort.signal.aborted || at !== epoch.current) break;
          setValues(old => ({ ...old, [p.id]: value }));
        } catch (e) {
          if (abort.signal.aborted || at !== epoch.current) break;
          const code = e instanceof ProviderProblem ? e.code : 'read_error';
          setErrors(old => ({ ...old, [p.id]: problemText[code] }));
        }
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); setPending(''); } }
  }
  return { values, errors, busy, pending, refresh };
}
export type MappingRatesController = ReturnType<typeof useMappingRates>;
