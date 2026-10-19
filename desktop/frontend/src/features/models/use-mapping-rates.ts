import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from '../providers/provider.model';
import type { KeyStatus } from '../../desktop/contract.generated';
import type { AccountRateObservation } from '../providers/provider-account.model';

export type RateTarget = { provider_id: string; key_id: string };
export const rateID = (t: RateTarget) => JSON.stringify([t.provider_id, t.key_id]);
/** Explicit reads per provider/Key pair, serialized and bound to the configuration. */
export function useMappingRates(providers: Provider[], active: boolean, transport: ProviderTransport) {
  const [values, setValues] = useState<Record<string, AccountRateObservation>>({});
  const [keys, setKeys] = useState<Record<string, KeyStatus>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState('');
  const lock = useRef(false), epoch = useRef(0), abortRef = useRef<AbortController | null>(null);
  const signature = JSON.stringify(providers.map(p => [p.id, p.revision]));
  useEffect(() => {
    const at = ++epoch.current;
    abortRef.current?.abort(); lock.current = false;
    setValues({}); setKeys({}); setErrors({}); setBusy(false); setPending('');
    return () => { epoch.current = at + 1; abortRef.current?.abort(); lock.current = false; };
  }, [signature, active, transport]);
  async function refresh(input: RateTarget[]) {
    if (!active || lock.current) return;
    const seen = new Set<string>();
    const targets = input.filter(t => { const id = rateID(t); if (seen.has(id) || !providers.some(p => p.id === t.provider_id)) return false; seen.add(id); return true; });
    lock.current = true; setBusy(true);
    const at = epoch.current, abort = new AbortController(); abortRef.current = abort;
    try {
      for (const target of targets) {
        const p = providers.find(p => p.id === target.provider_id)!; const id = rateID(target);
        if (abort.signal.aborted || at !== epoch.current) break;
        setPending(id);
        setErrors(old => { const next = { ...old }; delete next[id]; return next; });
        try {
          if (!transport.rate) throw new ProviderProblem('read_error');
          const value = await transport.rate({ provider_id: p.id, expected_revision: p.revision, operation: 'effective-rate', key_id: target.key_id, remote_key_id: 0 }, abort.signal);
          if (abort.signal.aborted || at !== epoch.current) break;
          setValues(old => ({ ...old, [id]: value }));
          if (!transport.workspace) throw new ProviderProblem('read_error');
          const workspace = await transport.workspace(p.id, abort.signal);
          if (abort.signal.aborted || at !== epoch.current) break;
          if (workspace.revision !== p.revision) throw new ProviderProblem('conflict');
          setKeys(old => ({ ...old, ...Object.fromEntries(workspace.keys.map(k => [rateID({ provider_id: p.id, key_id: k.id }), k])) }));
        } catch (e) {
          if (abort.signal.aborted || at !== epoch.current) break;
          const code = e instanceof ProviderProblem ? e.code : 'read_error';
          setErrors(old => ({ ...old, [id]: problemText[code] }));
        }
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); setPending(''); } }
  }
  return { values, keys, errors, busy, pending, refresh };
}
export type MappingRatesController = ReturnType<typeof useMappingRates>;
