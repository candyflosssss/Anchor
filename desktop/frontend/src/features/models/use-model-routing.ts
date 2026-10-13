import { useEffect, useRef, useState } from 'react';
import type { ModelGroup } from '../providers/provider-summary.model';
import { ProviderProblem, problemText, type ProviderTransport } from '../providers/provider.model';
import { routeID, moveRoute, type RouteCandidate } from './model-routing.model';
type Draft = { strategy: string; order: RouteCandidate[] };
const original = (g: ModelGroup): Draft => ({ strategy: g.strategy, order: g.candidates.map(c => ({ provider_id: c.provider_id, model: c.upstream })) });
export function useModelRouting(groups: ModelGroup[], active: boolean, transport: ProviderTransport, reload: () => Promise<void>) {
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [needsRead, setNeedsRead] = useState(false);
  const epoch = useRef(0), lock = useRef(false), abort = useRef<AbortController | null>(null);
  const current = useRef(groups); current.current = groups;
  const signature = JSON.stringify(groups.map(g => [g.alias, g.revision, g.strategy, g.complete, g.candidates.map(c => [c.provider_id, c.upstream, c.key_ids])]));
  useEffect(() => {
    const lifecycle = { epoch, abort, lock };
    const cancel = () => { lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false; };
    cancel(); setDrafts({}); setBusy(false); setNeedsRead(false);
    return cancel;
  }, [signature, active, transport]);
  function draft(g: ModelGroup) { return drafts[g.alias] ?? original(g); }
  function change(g: ModelGroup, value: Partial<Draft>) {
    if (!active || lock.current || !g.complete || needsRead) return;
    setDrafts(old => ({ ...old, [g.alias]: { ...(old[g.alias] ?? original(g)), ...value } })); setNotice(''); setError('');
  }
  function move(g: ModelGroup, from: number, to: number) { change(g, { order: moveRoute(draft(g).order, from, to) }); }
  function dirty(g: ModelGroup) { return JSON.stringify(draft(g)) !== JSON.stringify(original(g)); }
  async function save(g: ModelGroup) {
    if (!active || lock.current || !g.complete || needsRead || !dirty(g)) return;
    const latest = current.current.find(v => v.alias === g.alias);
    if (!latest || latest.revision !== g.revision) return;
    lock.current = true; setBusy(true); setError(''); setNotice(''); const at = epoch.current, request = new AbortController(); abort.current = request;
    const value = draft(g);
    try {
      if (!transport.routing) throw new ProviderProblem('invalid');
      await transport.routing({ model_id: g.alias, expected_revision: g.revision, strategy: value.strategy, order: value.order }, request.signal);
      if (request.signal.aborted || at !== epoch.current) return;
      setNotice(`${g.alias} 的顺序与调度规则已保存。`);
      await reload();
    } catch (e) {
      if (request.signal.aborted || at !== epoch.current) return;
      const code = e instanceof ProviderProblem ? e.code : 'unknown'; setError(problemText[code]);
      if (code === 'conflict' || code === 'unknown') { setNeedsRead(true); setDrafts({}); await reload(); }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  async function refresh() {
    if (lock.current) return; lock.current = true; setBusy(true); const at = epoch.current;
    try { await reload(); if (at === epoch.current) { setNeedsRead(false); setDrafts({}); } }
    finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  return { draft, change, move, dirty, save, refresh, busy, error, notice, needsRead, routeID, reset: (g: ModelGroup) => change(g, original(g)) };
}
export type ModelRoutingController = ReturnType<typeof useModelRouting>;
