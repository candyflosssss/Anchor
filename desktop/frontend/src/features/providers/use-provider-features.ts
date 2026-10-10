import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { actionFor, type Action, type Workspace } from './provider-features.model';
export function useProviderFeatures(p: Provider | null, transport: ProviderTransport, changed: (id: string | null) => Promise<void>) {
  const [data, setData] = useState<Workspace | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const [notice, setNotice] = useState('');
  const query = useRef<AbortController | null>(null); const write = useRef<AbortController | null>(null); const lock = useRef(false);
  const epoch = useRef(0); const after = useRef(changed); after.current = changed;
  async function refresh() {
    if (!p) return;
    query.current?.abort(); const abort = new AbortController(); query.current = abort; const generation = epoch.current;
    setError('');
    try { if (!transport.workspace) throw new ProviderProblem('read_error'); const result = await transport.workspace(p.id, abort.signal); if (!abort.signal.aborted && generation === epoch.current) { setData(result); setNeedsRead(false); if (result.revision !== p.revision) await after.current(result.provider_id); } }
    catch (e) { if (!abort.signal.aborted && generation === epoch.current) setError(e instanceof ProviderProblem ? problemText[e.code] : '读取详情失败。'); }
  }
  useEffect(() => { epoch.current++; setData(null); setError(''); setNotice(''); setNeedsRead(false); void refresh(); return () => { query.current?.abort(); write.current?.abort(); }; }, [p?.id, p?.revision, transport]);
  async function run(action: string, fields: Partial<Action> = {}) {
    if (!p || lock.current || needsRead) return false;
    lock.current = true; setBusy(true); setError(''); setNotice(''); query.current?.abort(); const abort = new AbortController(); write.current = abort;
    const generation = epoch.current; const input = actionFor(p, action, fields);
    try {
      if (!transport.action) throw new ProviderProblem('invalid');
      const result = await transport.action(input, abort.signal);
      if (abort.signal.aborted || generation !== epoch.current) return false;
      setData(result); setNotice(action === 'test' ? '测试完成，结果见下方。' : action === 'price-import' ? '价格已读取，确认后保存才会生效。' : '操作已保存。');
      if (!['test', 'price-import'].includes(action)) await after.current(result.deleted ? null : result.provider_id);
      return true;
    } catch (e) {
      if (!abort.signal.aborted && generation === epoch.current) { const code = e instanceof ProviderProblem ? e.code : 'unknown'; setError(problemText[code]); if (code === 'unknown' || code === 'conflict') setNeedsRead(true); }
      return false;
    } finally { input.secret = ''; lock.current = false; setBusy(false); }
  }
  return { data, busy, isBusy: () => lock.current, error, notice, needsRead, refresh, run };
}
export type ProviderFeatures = ReturnType<typeof useProviderFeatures>;
