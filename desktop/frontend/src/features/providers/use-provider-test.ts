import { useEffect, useRef, useState } from 'react';
import type { ProbeProgress, ProbeRequest } from '../../desktop/contract.generated';
import { modelsForKey } from './provider-test.model';
import { ProviderProblem, type Provider, type ProviderTransport } from './provider.model';

/** An isolated dialog session: fixed mapping or Key-specific catalog, transient output and cancellation. */
export function useProviderTest(p: Provider, key: string, configuredModel: string, transport: ProviderTransport, finished: () => Promise<void>) {
  const [models, setModels] = useState<string[]>([]);
  const [model, setModel] = useState('');
  const [loading, setLoading] = useState(true);
  const [catalogError, setCatalogError] = useState('');
  const [retry, setRetry] = useState(0);
  const [progress, setProgress] = useState<ProbeProgress | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const live = useRef(true);
  const locked = useRef(false);
  const stopping = useRef(false);
  const run = useRef<ProbeRequest | null>(null);
  const request = useRef<AbortController | null>(null);
  const after = useRef(finished); after.current = finished;
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
      request.current?.abort();
      const current = run.current;
      if (current && transport.probe) void transport.probe({ ...current, action: 'cancel' }, new AbortController().signal).catch(() => {});
    };
  }, [transport]);
  useEffect(() => {
    const abort = new AbortController();
    setModels([]); setModel(''); setLoading(true); setCatalogError(''); setProgress(null); setError('');
    // A saved upstream model is authoritative; catalog permissions are not needed to probe it.
    if (configuredModel) {
      setModels([configuredModel]); setModel(configuredModel); setLoading(false);
      return () => abort.abort();
    }
    if (!key) { setLoading(false); return () => abort.abort(); }
    void transport.discover({ provider_id: p.id, expected_revision: p.revision, key_id: key }, abort.signal).then(catalog => {
      if (abort.signal.aborted) return;
      const options = modelsForKey(catalog, key);
      setModels(options); setModel(options[0] ?? '');
      if (!options.length) setCatalogError(catalog.failure ? '未能获取此 Key 的可用模型。请检查目录权限后重试。' : '此 Key 没有可用模型。');
    }).catch(e => {
      if (!abort.signal.aborted) setCatalogError(e instanceof ProviderProblem && e.code === 'conflict' ? '配置版本已变化，请关闭弹窗并刷新后重试。' : '未能获取此 Key 的可用模型，请重试。');
    }).finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [transport, p.id, p.revision, key, configuredModel, retry]);

  async function start() {
    if (locked.current || loading || !models.includes(model) || !key || !transport.probe) return;
    locked.current = true; stopping.current = false; setBusy(true); setError(''); setProgress(null);
    const abort = new AbortController(); request.current = abort;
    const input: ProbeRequest = { provider_id: p.id, expected_revision: p.revision, action: 'start', key_id: key, model, run_id: '', approved: true };
    try {
      let next = await transport.probe(input, new AbortController().signal);
      run.current = { ...input, run_id: next.run_id };
      if (!live.current || abort.signal.aborted) return;
      if (stopping.current) await transport.probe({ ...run.current, action: 'cancel' }, new AbortController().signal);
      setProgress(next);
      while (next.status === 'running' && !abort.signal.aborted) {
        await new Promise<void>(resolve => {
          const done = () => { clearTimeout(timer); abort.signal.removeEventListener('abort', done); resolve(); };
          const timer = setTimeout(done, 150); abort.signal.addEventListener('abort', done, { once: true });
        });
        if (abort.signal.aborted) break;
        next = await transport.probe({ ...run.current!, action: 'read' }, abort.signal);
        if (live.current && !abort.signal.aborted) setProgress(next);
      }
      run.current = null;
      if (live.current && !abort.signal.aborted) await after.current();
    } catch (e) {
      if (live.current && !abort.signal.aborted) setError(e instanceof ProviderProblem && e.code === 'conflict' ? '配置版本已变化，请刷新后重试。' : '测试返回中断。请核对结果后重试。');
    } finally {
      const current = run.current; run.current = null;
      if (current && transport.probe) void transport.probe({ ...current, action: 'cancel' }, new AbortController().signal).catch(() => {});
      locked.current = false;
      if (live.current) setBusy(false);
    }
  }
  async function stop() {
    stopping.current = true;
    const current = run.current;
    if (current && transport.probe) {
      try { await transport.probe({ ...current, action: 'cancel' }, new AbortController().signal); }
      catch { if (live.current) setError('未能确认停止状态，测试将在超时后结束。'); }
    }
  }
  return { models, model, setModel: (value: string) => { setModel(value); setProgress(null); setError(''); }, loading, catalogError, refreshCatalog: () => setRetry(n => n + 1), progress, error, busy, start, stop };
}
