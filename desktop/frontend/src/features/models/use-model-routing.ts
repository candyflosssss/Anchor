import { useEffect, useRef, useState } from 'react';
import type { ProbeProgress, ProbeRequest, SourceRoute } from '../../desktop/contract.generated';
import type { ModelGroup } from '../providers/provider-summary.model';
import { ProviderProblem, problemText, type ProviderTransport } from '../providers/provider.model';
import { checkedRouting, modelSources, sourceID, routeID, moveRoute, priceOrder, effectiveRouteMultiplier, type RouteCandidate } from './model-routing.model';
type Draft = { strategy: string; order: RouteCandidate[]; source_order: SourceRoute[] };
type Entry = { revision: number; value: Draft; saved: Draft; phase: 'saving' | 'saved' | 'failed' | 'uncertain'; message: string; undo?: Draft };
const original = (g: ModelGroup): Draft => ({ source_order: modelSources(g).map(v => v.ref), strategy: g.strategy, order: g.candidates.map(c => ({ provider_id: c.provider_id, model: c.upstream })) });
const same = (a: Draft, b: Draft) => JSON.stringify(a) === JSON.stringify(b);

/** Changes are written once, immediately; uncertain writes require an explicit read. */
export function useModelRouting(groups: ModelGroup[], active: boolean, transport: ProviderTransport, reload: () => Promise<void>) {
  const [entries, setEntries] = useState<Record<string, Entry>>({});
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [needsRead, setNeedsRead] = useState(false);
  const [sorting, setSorting] = useState<{ alias: string; complete: number; total: number; failures: number } | null>(null);
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
    cancel(); lifecycle.latestEntries.current = {}; setEntries({}); setBusy(false); setNeedsRead(false); setError(''); setSorting(null);
    return cancel;
  }, [active, transport]);
  function entry(g: ModelGroup) {
    const v = latestEntries.current[g.alias];
    // A newer complete snapshot from another writer wins. Partial background
    // reads cannot replace the acknowledged order of an optimistic update.
    return v && (!g.complete || g.revision <= v.revision) ? v : undefined;
  }
  function draft(g: ModelGroup) { return entry(g)?.value ?? original(g); }
  async function write(g: ModelGroup, value: Draft, undoing = false, heldLock = false) {
    if (!active || (lock.current && !heldLock) || !g.complete || needsRead) return;
    const latest = current.current.find(v => v.alias === g.alias);
    if (!latest || !latest.complete || latest.revision !== g.revision) return;
    const old = entry(latest), saved = old?.saved ?? original(latest), revision = old?.revision ?? latest.revision;
    if (same(value, saved)) return;
    lock.current = true; setBusy(true); setError('');
    const at = epoch.current, request = new AbortController(); abort.current = request;
    put(g.alias, { revision, value, saved, phase: 'saving', message: '正在保存…' });
    try {
      if (!transport.routing) throw new ProviderProblem('invalid');
      const input = { model_id: g.alias, expected_revision: revision, strategy: value.strategy, order: value.order, source_order: value.source_order };
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
  function move(g: ModelGroup, from: number, to: number) {
    if (from === to || to < 0 || to >= draft(g).source_order.length) return;
    return change(g, { strategy: 'priority', source_order: moveRoute(draft(g).source_order, from, to) });
  }
  function orderedSources(g: ModelGroup) { return modelSources(g, draft(g).source_order); }
  function sortByPrice(g: ModelGroup) {
    const source_order = priceOrder(orderedSources(g).map(v => ({ ...v, provider_id:v.ref.provider_id, model:v.ref.model })), v => effectiveRouteMultiplier({ ...v.candidate, key_ids: [v.source.id] })).map(v => v.ref);
    return change(g, { strategy: 'priority', source_order });
  }
  async function sortByLatency(g: ModelGroup) {
    const probe = transport.probe?.bind(transport);
    if (!active || lock.current || !g.complete || needsRead || !probe) return;
    const baseline = orderedSources(g);
    const targets = baseline.filter(({ candidate, source }) => candidate.availability.state === 'available' && (source.anonymous || source.key?.enabled && ['ready', 'degraded'].includes(source.key.state))).map(v => ({ ...v, key:v.source.id }));
    if (!targets.length) return;
    lock.current = true; setBusy(true); setError('');
    const at = epoch.current, request = new AbortController(); abort.current = request;
    const timings = new Map<string, number>(); let failures = 0;
    setSorting({ alias: g.alias, complete: 0, total: targets.length, failures: 0 });
    try {
      for (let i = 0; i < targets.length; i++) {
        const { candidate, key, ref } = targets[i];
        if (request.signal.aborted || at !== epoch.current) return;
        let progress: ProbeProgress | undefined;
        let run: ProbeRequest | undefined;
        const targetAbort = new AbortController();
        const stopTarget = () => targetAbort.abort();
        request.signal.addEventListener('abort', stopTarget, { once: true });
        const deadline = setTimeout(stopTarget, 15_000);
        try {
          const input = { provider_id: candidate.provider_id, expected_revision: candidate.provider_revision, action: 'start', key_id: key, model: candidate.upstream, run_id: '', approved: true };
          progress = await probe(input, AbortSignal.timeout(15_000));
          run = { ...input, run_id: progress.run_id };
          while (progress.status === 'running' && !targetAbort.signal.aborted) {
            await new Promise<void>(resolve => {
              const done = () => { clearTimeout(timer); targetAbort.signal.removeEventListener('abort', done); resolve(); };
              const timer = setTimeout(done, 150); targetAbort.signal.addEventListener('abort', done, { once: true });
            });
            if (!targetAbort.signal.aborted) progress = await probe({ ...run, action: 'read' }, targetAbort.signal);
          }
          const firstByte = progress.first_byte_ms ?? 0;
          if (progress.status !== 'completed' || !progress.result || progress.result.failure || firstByte <= 0) failures++;
          else timings.set(sourceID(ref), firstByte);
        } catch {
          failures++;
        } finally {
          clearTimeout(deadline); request.signal.removeEventListener('abort', stopTarget);
          if (run && progress?.status === 'running') await probe({ ...run, action: 'cancel' }, AbortSignal.timeout(15_000)).catch(() => {});
        }
        if (at === epoch.current) setSorting({ alias: g.alias, complete: i + 1, total: targets.length, failures });
      }
      if (request.signal.aborted || at !== epoch.current) return;
      const currentGroup = current.current.find(v => v.alias === g.alias);
      if (!currentGroup?.complete || currentGroup.revision !== g.revision || g.candidates.some(c => currentGroup.candidates.find(v => v.provider_id === c.provider_id && v.upstream === c.upstream)?.provider_revision !== c.provider_revision)) { setNeedsRead(true); setError('配置已变化，请刷新后重试。'); return; }
      if (!timings.size) { setError('未测得首字延迟，顺序未修改。'); return; }
      const source_order = [...baseline].sort((a, b) => {
        const left = timings.get(sourceID(a.ref));
        const right = timings.get(sourceID(b.ref));
        if (left == null && right != null) return 1;
        if (left != null && right == null) return -1;
        return (left ?? 0) - (right ?? 0);
      }).map(v => v.ref);
      setSorting(null);
      await write(g, { ...draft(g), strategy: 'priority', source_order }, false, true);
    } finally {
      if (at === epoch.current) { lock.current = false; setBusy(false); setSorting(null); }
    }
  }
  function stopSorting() { if (sorting) abort.current?.abort(); }
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
  return { draft, change, move, sortByPrice, sortByLatency, stopSorting, sorting, canProbe: !!transport.probe, refresh, retry, undo, discard, status: entry, busy, error, needsRead, routeID };
}
export type ModelRoutingController = ReturnType<typeof useModelRouting>;
