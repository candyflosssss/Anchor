import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { actionFor, type Action, type Workspace } from './provider-features.model';

/** Reconcile after a write, conflict or detail revision mismatch; null id means deleted. */
export type FeatureChanged = (id: string | null, action: string, from: string) => Promise<void>;

export function useProviderFeatures(p: Provider | null, transport: ProviderTransport, changed: FeatureChanged, started?: () => void, readVersion = 0) {
  const [data, setData] = useState<Workspace | null>(null);
  const [busy, setBusy] = useState(false);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const [notice, setNotice] = useState('');
  const query = useRef<AbortController | null>(null); const write = useRef<AbortController | null>(null); const lock = useRef(false);
  const epoch = useRef(0); const lastId = useRef<string | null>(null);
  const recoveringConflict = useRef(false);
  const after = useRef(changed); after.current = changed;
  const begin = useRef(started); begin.current = started;
  const current = useRef(p); current.current = p;

  async function refresh() {
    const target = current.current;
    if (!target) return;
    query.current?.abort(); const abort = new AbortController(); query.current = abort; const generation = epoch.current;
    setReading(true); setError('');
    try {
      if (!transport.workspace) throw new ProviderProblem('read_error');
      const result = await transport.workspace(target.id, abort.signal);
      if (abort.signal.aborted || generation !== epoch.current) return;
      setData(result); setNeedsRead(false);
      if (recoveringConflict.current) {
        recoveringConflict.current = false;
        setNotice('已同步最新配置。刚才的操作未保存，请核对后重新操作。');
      }
      if (result.revision !== target.revision) await after.current(result.provider_id, 'refresh', target.id);
    } catch (e) {
      if (!abort.signal.aborted && generation === epoch.current) setError(e instanceof ProviderProblem ? problemText[e.code] : '读取详情失败。');
    } finally {
      if (query.current === abort) { query.current = null; setReading(false); }
    }
  }

  // Every successful list read also refreshes details, even when revision is unchanged.
  // Model conflicts and unknown non-config actions need not bump the provider revision.
  const refreshRef = useRef(refresh); refreshRef.current = refresh;
  const id = p?.id ?? null; const revision = p?.revision;
  useEffect(() => {
    if (id !== lastId.current) {
      lastId.current = id; epoch.current++;
      query.current?.abort(); query.current = null; setReading(false); write.current?.abort(); lock.current = false;
      recoveringConflict.current = false;
      setBusy(false); setData(null); setError(''); setNotice(''); setNeedsRead(false);
      void refreshRef.current();
    } else if (id) void refreshRef.current();
  }, [id, revision, transport, readVersion]);
  useEffect(() => () => { query.current?.abort(); write.current?.abort(); }, []);

  async function run(action: string, fields: Partial<Action> = {}) {
    const target = current.current;
    if (!target || lock.current || query.current || needsRead) return false;
    begin.current?.();
    lock.current = true; setBusy(true); setError(''); setNotice(''); const abort = new AbortController(); write.current = abort;
    const generation = epoch.current; const input = actionFor(target, action, fields);
    try {
      if (!transport.action) throw new ProviderProblem('invalid');
      const result = await transport.action(input, abort.signal);
      if (abort.signal.aborted || generation !== epoch.current) return false;
      setData(result); setNotice(action === 'test' ? '测试完成，结果见下方。' : action === 'price-import' ? '价格已读取，确认后保存才会生效。' : '操作已保存。');
      if (!['test', 'price-import'].includes(action)) await after.current(result.deleted ? null : result.provider_id, action, target.id);
      return true;
    } catch (e) {
      if (!abort.signal.aborted && generation === epoch.current) {
        const code = e instanceof ProviderProblem ? e.code : 'unknown';
        setError(problemText[code]);
        if (code === 'unknown' || code === 'conflict') setNeedsRead(true);
        if (code === 'conflict') {
          // A rejected write is safe to follow with reads, never with a replay.
          recoveringConflict.current = true;
          await after.current(target.id, 'conflict', target.id);
        }
      }
      return false;
    } finally {
      input.secret = '';
      if (generation === epoch.current) { lock.current = false; setBusy(false); }
    }
  }
  const visibleData = data && p && data.provider_id === p.id ? data : null;
  return { data: visibleData, busy: busy || reading, isBusy: () => lock.current, error, notice, needsRead, refresh, run };
}
export type ProviderFeatures = ReturnType<typeof useProviderFeatures>;
