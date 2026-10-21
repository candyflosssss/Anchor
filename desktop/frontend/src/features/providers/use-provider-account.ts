import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { userTokenError } from './provider-account.model';
import type { AccountSettings, AccountObservation, AccountRateObservation, SaveAccountSettings, AccountQuery } from './provider-account.model';

export function useProviderAccount(p: Provider | null, transport: ProviderTransport, changed: (settings: AccountSettings) => Promise<void>, blocked: () => boolean, readVersion: number, autoQuery = false, observed?: () => Promise<void>) {
  const [settings, setSettings] = useState<AccountSettings | null>(null);
  const [observedPlatform, setObservedPlatform] = useState('');
  const [balance, setBalance] = useState<AccountObservation | null>(null);
  const [quota, setQuota] = useState<AccountObservation | null>(null);
  const [rate, setRate] = useState<AccountRateObservation | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const lock = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const epoch = useRef(0);
  const id = p?.id, revision = p?.revision;
  const autoQueried = useRef(false);
  const retry = useRef<{ id: string; revision: number; key: string } | null>(null);
  const lastKey = useRef('');
  const scope = useRef<{ id?: string; revision?: number; transport: ProviderTransport } | null>(null);
  useEffect(() => {
    const abort = new AbortController(); abortRef.current?.abort(); abortRef.current = abort;
    const at = ++epoch.current;
    const previous = scope.current; scope.current = { id, revision, transport };
    if (previous?.id !== id || previous?.transport !== transport) { setSettings(null); lastKey.current = ''; retry.current = null; }
    if (previous?.id !== id || previous?.transport !== transport || previous?.revision !== revision) { setObservedPlatform(''); setBalance(null); setQuota(null); setRate(null); }
    autoQueried.current = false;
    setError(''); setNeedsRead(false);
    lock.current = !!id; setBusy(!!id);
    if (id) {
      void (async () => {
        try {
          if (!transport.accountSettings) throw new ProviderProblem('read_error');
          const v = await transport.accountSettings(id, abort.signal);
          if (abort.signal.aborted || at !== epoch.current) return;
          if (v.revision !== revision) throw new ProviderProblem('conflict');
          setSettings(v); setObservedPlatform(v.platform && v.platform !== 'auto' ? v.platform : v.detected_platform);
        } catch (e) {
          if (!abort.signal.aborted && at === epoch.current) {
            const code = e instanceof ProviderProblem ? e.code : 'read_error'; setError(problemText[code]); setNeedsRead(true);
          }
        } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
      })();
    }
    return () => { abort.abort(); abortRef.current?.abort(); epoch.current = at + 1; lock.current = false; };
  }, [id, revision, readVersion, transport]);
  const matches = !!settings && settings.provider_id === id && settings.revision === revision;
  async function run(operation: 'detect-platform' | 'save' | 'balance' | 'key-quota' | 'effective-rate', input: SaveAccountSettings | AccountQuery) {
    if (!p || !matches || lock.current || blocked() || needsRead) return false;
    if (input.provider_id !== p.id || input.expected_revision !== p.revision) return false;
    lock.current = true; setBusy(true); setError('');
    if (operation === 'save') { setBalance(null); setQuota(null); setRate(null); }
    const at = epoch.current, abort = new AbortController(); abortRef.current = abort;
    try {
      if (operation === 'detect-platform') {
        if (!transport.detectPlatform) throw new ProviderProblem('invalid');
        const v = await transport.detectPlatform(input as AccountQuery, abort.signal);
        if (abort.signal.aborted || at !== epoch.current) return false;
        if (v.revision !== p.revision) throw new ProviderProblem('conflict');
        setSettings(v); setObservedPlatform(v.platform && v.platform !== 'auto' ? v.platform : v.detected_platform);
        setBalance(null); setQuota(null); setRate(null);
        await observed?.();
        return true;
      }
      if (operation === 'save') {
        if (!transport.saveAccount) throw new ProviderProblem('invalid');
        const v = await transport.saveAccount(input as SaveAccountSettings, abort.signal);
        if (abort.signal.aborted || at !== epoch.current) return false;
        if (v.revision !== p.revision + 1) throw new ProviderProblem('unknown');
        if ((input as SaveAccountSettings).user_token) retry.current = { id: p.id, revision: v.revision, key: lastKey.current };
        await changed(v);
        return true;
      }
      const read = operation === 'effective-rate' ? transport.rate?.bind(transport) : transport.allowance?.bind(transport);
      if (!read) throw new ProviderProblem('read_error');
      const v = await read(input as AccountQuery, abort.signal);
      if (abort.signal.aborted || at !== epoch.current) return false;
      if (settings?.platform && settings.platform !== 'auto' && v.platform !== settings.platform) throw new ProviderProblem('read_error');
      if (['sub2api', 'newapi', 'oneapi'].includes(v.platform)) setObservedPlatform(v.platform);
      // A legacy supplier may have been identified by this first query. Read
      // its saved result locally; do not infer detection failure from a rate failure.
      if (settings && !settings.detected_platform && !settings.detection_failure && (!settings.platform || settings.platform === 'auto') && transport.accountSettings) {
        const saved = await transport.accountSettings(p.id, abort.signal);
        if (abort.signal.aborted || at !== epoch.current) return false;
        if (saved.revision !== p.revision) throw new ProviderProblem('conflict');
        setSettings(saved);
      }
      if ('rate' in v) {
        lastKey.current = (input as AccountQuery).key_id; setRate(v);
        if (!v.failure) await observed?.();
      } else if (operation === 'balance') setBalance(v); else setQuota(v);
      return true;
    } catch (e) {
      if (abort.signal.aborted || at !== epoch.current) return false;
      const writing = operation === 'save' || operation === 'detect-platform';
      const code = e instanceof ProviderProblem ? e.code : writing ? 'unknown' : 'read_error';
      setError(problemText[code]); setNeedsRead(code === 'conflict' || (writing && code === 'unknown'));
      return false;
    } finally {
      if ('user_token' in input) input.user_token = null;
      if (at === epoch.current) { lock.current = false; setBusy(false); }
    }
  }
  const runRef = useRef(run); runRef.current = run;
  useEffect(() => {
    if (!id || !matches || !autoQuery || busy || needsRead || autoQueried.current) return;
    const pending = retry.current;
    const retrying = pending?.id === id && pending.revision === revision;
    autoQueried.current = true;
    if (retrying) retry.current = null;
    // A single read per selection/revision, deferred until catalog and writes are idle.
    void runRef.current('effective-rate', { provider_id: id, expected_revision: revision!, operation: 'effective-rate', key_id: retrying ? pending.key : '', remote_key_id: 0 });
  }, [id, revision, matches, autoQuery, busy, needsRead, settings, readVersion, transport]);
  return { observedPlatform: matches ? observedPlatform : '', ready: matches, settings: settings?.provider_id === id ? settings : null, balance: matches ? balance : null, quota: matches ? quota : null, rate: matches ? rate : null, busy, error, needsRead,
    isBusy: () => lock.current,
    detectPlatform: (key = '') => p ? run('detect-platform', { provider_id: p.id, expected_revision: p.revision, operation: 'detect-platform', key_id: key, remote_key_id: 0 }) : Promise.resolve(false),
    save: (input: SaveAccountSettings) => run('save', input),
    authorizeNewAPI: async (token: string, userID: string) => {
      const uid = userID.trim() || '0';
      const invalid = userTokenError(token) || (!/^\d+$/.test(uid) || !Number.isSafeInteger(Number(uid)) ? '用户 ID 必须为安全范围内的非负整数。' : '');
      if (invalid) { setError(invalid); return false; }
      if (!settings || !matches || (settings.platform !== 'newapi' && observedPlatform !== 'newapi')) return false;
      return run('save', { provider_id: settings.provider_id, expected_revision: settings.revision, platform: settings.platform || 'auto', site_url: settings.site_url, user_id: Number(uid), user_token: token.trim() });
    },
    query: (input: AccountQuery) => run(input.operation as 'balance' | 'key-quota' | 'effective-rate', input),
    clearObservations: () => { if (!lock.current) { setQuota(null); setBalance(null); setRate(null); } },
  };
}
export type ProviderAccountController = ReturnType<typeof useProviderAccount>;
