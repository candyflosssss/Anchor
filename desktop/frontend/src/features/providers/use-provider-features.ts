import { useEffect, useRef, useState } from 'react';
import { ProviderProblem, problemText, type Provider, type ProviderTransport } from './provider.model';
import { actionFor, catalogImportPlan, type Action, type Workspace } from './provider-features.model';

/** Reconcile after a write, conflict or detail revision mismatch; null id means deleted. */
export type FeatureChanged = (id: string | null, action: string, from: string) => Promise<void>;

export function useProviderFeatures(p: Provider | null, transport: ProviderTransport, changed: FeatureChanged, started?: () => void, readVersion = 0) {
  const [data, setData] = useState<Workspace | null>(null);
  const [busy, setBusy] = useState(false);
  const [reading, setReading] = useState(false);
  const [error, setError] = useState('');
  const [needsRead, setNeedsRead] = useState(false);
  const [notice, setNotice] = useState('');
  const [importStatus, setImportStatus] = useState('');
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
      setBusy(false); setData(null); setImportStatus(''); setError(''); setNotice(''); setNeedsRead(false);
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
  async function importModels(ids: string[], sources: Record<string, string[]> = {}) {
    const target = current.current;
    if (!target || !data || data.provider_id !== target.id || lock.current || query.current || needsRead) return;
    const plan = catalogImportPlan(ids, data);
    const skipped = `跳过 ${plan.mapped} 个已映射模型，${plan.manual.length} 个需自定义别名。`;
    if (!plan.ready.length) { setImportStatus(`没有可快速导入的模型。${skipped}`); return; }
    begin.current?.();
    lock.current = true; setBusy(true); setError(''); setNotice('');
    const abort = new AbortController(); write.current = abort;
    const generation = epoch.current;
    let completed = 0;
    let snapshot = data;
    let failure = '';
    try {
      if (!transport.action) throw new ProviderProblem('invalid');
      for (const model of plan.ready) {
        setImportStatus(`正在导入 ${completed + 1} / ${plan.ready.length}：${model}`);
        snapshot = await transport.action(actionFor({ ...target, revision: snapshot.revision }, 'model-put', {
          target: model, model, key_ids: sources[model] ?? [], multiplier: 1, tools: true, streaming: true,
        }), abort.signal);
        if (abort.signal.aborted || generation !== epoch.current) return;
        completed++;
        setData(snapshot);
      }
    } catch (e) {
      if (abort.signal.aborted || generation !== epoch.current) return;
      const code = e instanceof ProviderProblem ? e.code : 'unknown';
      setError(problemText[code]);
      setNeedsRead(true);
      failure = `在「${plan.ready[completed]}」处停止：${problemText[code]} 请刷新核对后继续。`;
    } finally {
      if (!abort.signal.aborted && generation === epoch.current) {
        // Reconcile once for the whole batch; never replay a failed/unknown write.
        if (!failure) await after.current(target.id, 'model-put', target.id);
        setImportStatus(`已导入 ${completed} / ${plan.ready.length} 个。${skipped}${failure}`);
        lock.current = false; setBusy(false);
      }
    }
  }

  const visibleData = data && p && data.provider_id === p.id ? data : null;
  return { data: visibleData, busy: busy || reading, isBusy: () => lock.current, error, notice, needsRead, refresh, run, importModels, importStatus };
}
export type ProviderFeatures = ReturnType<typeof useProviderFeatures>;
