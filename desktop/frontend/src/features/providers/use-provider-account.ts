import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import type { AccountSettings, AccountObservation, SaveAccountSettings, AccountQuery } from './provider-account.model';

export function useProviderAccount(p: Provider | null, transport: ProviderTransport, changed: () => Promise<void>, blocked: () => boolean, readVersion: number) {
  const [settings, setSettings] = useState<AccountSettings | null>(null);
  const [balance, setBalance] = useState<AccountObservation | null>(null);
  const [quota, setQuota] = useState<AccountObservation | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const lock = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const epoch = useRef(0);
  const id = p?.id, revision = p?.revision;
  useEffect(() => {
    const abort = new AbortController(); abortRef.current?.abort(); abortRef.current = abort;
    const at = ++epoch.current;
    setSettings(null); setBalance(null); setQuota(null); setError(''); setNeedsRead(false);
    lock.current = !!id; setBusy(!!id);
    if (id) {
      void (async () => {
        try {
          if (!transport.accountSettings) throw new ProviderProblem('read_error');
          const v = await transport.accountSettings(id, abort.signal);
          if (abort.signal.aborted || at !== epoch.current) return;
          if (v.revision !== revision) throw new ProviderProblem('conflict');
          setSettings(v);
        } catch (e) {
          if (!abort.signal.aborted && at === epoch.current) {
            const code = e instanceof ProviderProblem ? e.code : 'read_error'; setError(problemText[code]); setNeedsRead(code === 'conflict');
          }
        } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
      })();
    }
    return () => { abort.abort(); abortRef.current?.abort(); epoch.current = at + 1; lock.current = false; };
  }, [id, revision, readVersion, transport]);
  const matches = !!settings && settings.provider_id === id && settings.revision === revision;
  async function run(operation: 'save' | 'balance' | 'key-quota', input: SaveAccountSettings | AccountQuery) {
    if (!p || !matches || lock.current || blocked() || needsRead) return false;
    if (input.provider_id !== p.id || input.expected_revision !== p.revision) return false;
    lock.current = true; setBusy(true); setError('');
    if (operation === 'save' || operation === 'balance') setBalance(null); if (operation === 'save' || operation === 'key-quota') setQuota(null);
    const at = epoch.current, abort = new AbortController(); abortRef.current = abort;
    try {
      if (operation === 'save') {
        if (!transport.saveAccount) throw new ProviderProblem('invalid');
        const v = await transport.saveAccount(input as SaveAccountSettings, abort.signal);
        if (abort.signal.aborted || at !== epoch.current) return false;
        if (v.revision !== p.revision + 1) throw new ProviderProblem('unknown');
        await changed();
        return true;
      }
      if (!transport.allowance) throw new ProviderProblem('read_error');
      const v = await transport.allowance(input as AccountQuery, abort.signal);
      if (abort.signal.aborted || at !== epoch.current) return false;
      if (v.platform !== settings?.platform) throw new ProviderProblem('read_error');
      if (operation === 'balance') setBalance(v); else setQuota(v);
      return true;
    } catch (e) {
      if (abort.signal.aborted || at !== epoch.current) return false;
      const code = e instanceof ProviderProblem ? e.code : operation === 'save' ? 'unknown' : 'read_error';
      setError(problemText[code]); setNeedsRead(code === 'conflict' || (operation === 'save' && code === 'unknown'));
      return false;
    } finally {
      if ('user_token' in input) input.user_token = null;
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    }
  }
  return { settings: matches ? settings : null, balance: matches ? balance : null, quota: matches ? quota : null, busy, error, needsRead,
    isBusy: () => lock.current,
    save: (input: SaveAccountSettings) => run('save', input),
    query: (input: AccountQuery) => run(input.operation as 'balance' | 'key-quota', input),
    clearObservations: () => { if (!lock.current) { setQuota(null); setBalance(null); } },
  };
}
export type ProviderAccountController = ReturnType<typeof useProviderAccount>;
