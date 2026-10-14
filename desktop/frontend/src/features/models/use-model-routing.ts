import { useEffect, useRef, useState } from 'react';
import type { ModelGroup } from '../providers/provider-summary.model';
import { ProviderProblem, problemText, type ProviderTransport } from '../providers/provider.model';
import { checkedRouting, routeID, moveRoute, type RouteCandidate } from './model-routing.model';
type Draft = { strategy: string; order: RouteCandidate[] };
type Entry = { revision: number; value: Draft; saved: Draft; phase: 'saving' | 'saved' | 'failed' | 'uncertain'; message: string; undo?: Draft };
const original = (g: ModelGroup): Draft => ({ strategy: g.strategy, order: g.candidates.map(c => ({ provider_id: c.provider_id, model: c.upstream })) });
const same = (a: Draft, b: Draft) => JSON.stringify(a) === JSON.stringify(b);

/** Changes are written once, immediately; uncertain writes require an explicit read. */
export function useModelRouting(groups: ModelGroup[], active: boolean, transport: ProviderTransport, reload: () => Promise<void>) {
  const [entries, setEntries] = useState<Record<string, Entry>>({});
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [needsRead, setNeedsRead] = useState(false);
  const epoch = useRef(0), lock = useRef(false), abort = useRef<AbortController | null>(null);
  const current = useRef(groups); current.current = groups;
  const latestEntries = useRef(entries); latestEntries.current = entries;
  function put(id: string, entry: Entry) {
    latestEntries.current = { ...latestEntries.current, [id]: entry };
    setEntries(latestEntries.current);
  }
  useEffect(() => {
    const lifecycle = { epoch, abort, lock, latestEntries };
    const cancel = () => { lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false; };
    cancel(); lifecycle.latestEntries.current = {}; setEntries({}); setBusy(false); setNeedsRead(false); setError('');
    return cancel;
  }, [active, transport]);
  function entry(g: ModelGroup) {
    const v = latestEntries.current[g.alias];
    // A newer complete snapshot from another writer wins. Partial background
    // reads cannot replace the acknowledged order of an optimistic update.
    return v && (!g.complete || g.revision <= v.revision) ? v : undefined;
  }
  function draft(g: ModelGroup) { return entry(g)?.value ?? original(g); }
  async function write(g: ModelGroup, value: Draft, undoing = false) {
    if (!active || lock.current || !g.complete || needsRead) return;
    const latest = current.current.find(v => v.alias === g.alias);
    if (!latest || !latest.complete || latest.revision !== g.revision) return;
    const old = entry(latest), saved = old?.saved ?? original(latest), revision = old?.revision ?? latest.revision;
    if (same(value, saved)) return;
    lock.current = true; setBusy(true); setError('');
    const at = epoch.current, request = new AbortController(); abort.current = request;
    put(g.alias, { revision, value, saved, phase: 'saving', message: '正在保存…' });
    try {
      if (!transport.routing) throw new ProviderProblem('invalid');
      const input = { model_id: g.alias, expected_revision: revision, strategy: value.strategy, order: value.order };
      const result = checkedRouting(await transport.routing(input, request.signal), input);
      if (request.signal.aborted || at !== epoch.current) return;
      put(g.alias, { revision: result.revision, value, saved: value, phase: 'saved', message: undoing ? '已撤销并保存' : '已自动保存', undo: undoing ? undefined : saved });
      // Persistence has already been acknowledged. A read failure must never
      // turn this successful write into a retryable write failure.
      try { await reload(); } catch { if (!request.signal.aborted && at === epoch.current) setError('修改已保存，配置读取失败，请刷新配置。'); }
    } catch (e) {
      if (request.signal.aborted || at !== epoch.current) return;
      const code = e instanceof ProviderProblem ? e.code : 'unknown';
      const uncertain = code === 'conflict' || code === 'unknown';
      put(g.alias, { revision, value: uncertain ? saved : value, saved, phase: uncertain ? 'uncertain' : 'failed', message: problemText[code] });
      if (uncertain) setNeedsRead(true);
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  function change(g: ModelGroup, value: Partial<Draft>) { return write(g, { ...draft(g), ...value }); }
  function move(g: ModelGroup, from: number, to: number) { return change(g, { order: moveRoute(draft(g).order, from, to) }); }
  function retry(g: ModelGroup) { const v = entry(g); if (v?.phase === 'failed') return write(g, v.value); }
  function undo(g: ModelGroup) { const v = entry(g); if (v?.undo) return write(g, v.undo, true); }
  function discard(g: ModelGroup) {
    const v = entry(g);
    if (!lock.current && v?.phase === 'failed') put(g.alias, { ...v, value: v.saved, phase: 'saved', message: '已放弃未保存的修改' });
  }
  async function refresh() {
    if (!active || lock.current) return;
    lock.current = true; setBusy(true); setError(''); const at = epoch.current;
    try {
      await reload();
      if (at === epoch.current) { latestEntries.current = {}; setEntries({}); setNeedsRead(false); }
    } catch { if (at === epoch.current) setError('配置读取失败，请重试。'); }
    finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  return { draft, change, move, refresh, retry, undo, discard, status: entry, busy, error, needsRead, routeID };
}
export type ModelRoutingController = ReturnType<typeof useModelRouting>;
