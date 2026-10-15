import { useEffect, useRef, useState } from 'react';
import { checkedWorkspace, type Workspace } from '../providers/provider-features.model';
import { ProviderProblem, problemText, type ProviderTransport } from '../providers/provider.model';
import { speedSettingsAction, speedSettingsError } from './model-speed-settings.model';

export function useModelSpeedSettings(id: string, provider: string, active: boolean, transport: ProviderTransport, saved: () => Promise<void>) {
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [tier, setTier] = useState(''), [variants, setVariants] = useState<string[]>([]);
  const [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const epoch = useRef(0), lock = useRef(false), abort = useRef<AbortController | null>(null);
  function accept(w: Workspace) {
    const m = w.models.find(m => m.id === id);
    if (w.provider_id !== provider || w.deleted || !m) throw new ProviderProblem('read_error');
    setWorkspace(w); setTier(m.service_tier ?? ''); setVariants([]); setNeedsRead(false);
  }
  async function run(work: (signal: AbortSignal) => Promise<void>) {
    if (!active || !provider || lock.current) return;
    lock.current = true; setBusy(true); setError(''); setNotice('');
    const at = epoch.current, request = new AbortController(); abort.current = request;
    try { await work(request.signal); }
    catch (e) {
      if (!request.signal.aborted && at === epoch.current) {
        const code = e instanceof ProviderProblem ? e.code : 'unknown';
        setError(problemText[code]);
        if (['unknown', 'conflict'].includes(code)) setNeedsRead(true);
      }
    } finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  async function read() {
    await run(async signal => {
      setWorkspace(null);
      if (!transport.workspace) throw new ProviderProblem('read_error');
      const w = checkedWorkspace(await transport.workspace(provider, signal));
      if (!signal.aborted) accept(w);
    });
  }
  const readRef = useRef(read); readRef.current = read;
  useEffect(() => {
    const lifecycle = { epoch, lock, abort };
    lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false;
    setWorkspace(null); setTier(''); setVariants([]); setBusy(false); setNeedsRead(false); setError(''); setNotice('');
    if (active && provider) void readRef.current();
    return () => { lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false; };
  }, [id, provider, active, transport]);
  const validation = workspace ? speedSettingsError(workspace, id, tier, variants) : '';
  async function save() {
    if (!workspace || workspace.provider_id !== provider || needsRead || validation) return;
    await run(async signal => {
      if (!transport.action) throw new ProviderProblem('invalid');
      const result = await transport.action(speedSettingsAction(workspace, id, tier, variants), signal);
      if (signal.aborted) return;
      // The write was acknowledged: never offer a blind retry if reconciliation fails.
      try { accept(checkedWorkspace(result)); } catch { setNeedsRead(true); setError('速度设置已保存，但返回配置无法核对，请重新读取。'); return; }
      setNotice('速度设置已保存；分身是独立映射。实际采用档位请查看请求日志中的上游返回。');
      try { await saved(); } catch { if (!signal.aborted) setError('速度设置已保存，模型列表刷新失败，请刷新配置。'); }
    });
  }
  function changeTier(value: string) { if (!lock.current && !needsRead) { setTier(value); setVariants([]); setNotice(''); } }
  function toggleVariant(value: string, checked: boolean) { if (!lock.current && !needsRead) { setVariants(v => checked ? [...v, value] : v.filter(s => s !== value)); setNotice(''); } }
  return { workspace: workspace?.provider_id === provider ? workspace : null, tier, variants, busy, needsRead, error, notice, validation, read, save, changeTier, toggleVariant };
}
