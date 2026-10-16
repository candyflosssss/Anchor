import { useEffect, useRef, useState } from 'react';
import type { Provider, ProviderTransport } from '../providers/provider.model';
import type { ProviderModels, Workspace } from '../../desktop/contract.generated';
import { chatFailure, chatMessages, chatModels, chatProblem, messageProblem, type ChatRequest, type ChatTransport, type ChatTurn } from './chat.model';

export function useChatOptions(transport: ProviderTransport) {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [providerId, setProviderId] = useState('');
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [keyId, setKeyId] = useState('');
  const [model, setModel] = useState('');
  const [catalog, setCatalog] = useState<ProviderModels | null>(null);
  const [loading, setLoading] = useState(true);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [error, setError] = useState('');
  const [catalogError, setCatalogError] = useState('');
  const [revision, setRevision] = useState(0);
  const [catalogRevision, setCatalogRevision] = useState(0);
  const provider = providers.find(p => p.id === providerId);
  useEffect(() => {
    const abort = new AbortController(); setLoading(true); setError('');
    void transport.list(abort.signal).then(rows => {
      if (abort.signal.aborted) return;
      const enabled = rows.filter(p => p.enabled); setProviders(enabled);
      setProviderId(current => enabled.some(p => p.id === current) ? current : enabled[0]?.id ?? '');
    }).catch(e => { if (!abort.signal.aborted) { setProviders([]); setError(chatProblem(e)); } })
      .finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [transport, revision]);
  useEffect(() => {
    const abort = new AbortController(); setWorkspace(null); setKeyId(''); setModel(''); setCatalog(null);
    if (!provider) return () => abort.abort();
    setLoading(true); setError('');
    void (transport.workspace ? transport.workspace(provider.id, abort.signal) : Promise.reject(new Error('unsupported'))).then(value => {
      if (abort.signal.aborted) return;
      setWorkspace(value); setKeyId(value.keys.length ? value.keys.find(k => k.enabled)?.id ?? '' : 'anonymous');
    }).catch(e => { if (!abort.signal.aborted) setError(chatProblem(e)); })
      .finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [transport, provider, revision]);
  useEffect(() => {
    const abort = new AbortController(); setCatalog(null); setCatalogError(''); setModel(''); setCatalogLoading(false);
    if (!workspace || workspace.provider_id !== providerId || !keyId) return () => abort.abort();
    const saved = chatModels(workspace, keyId, null); setModel(saved[0] ?? ''); setCatalogLoading(true);
    void transport.discover({ provider_id: workspace.provider_id, expected_revision: workspace.revision, key_id: keyId }, abort.signal).then(value => {
      if (abort.signal.aborted) return;
      setCatalog(value);
      setModel(current => current || chatModels(workspace, keyId, value)[0] || '');
      if (value.failure) setCatalogError('未能读取模型目录；仍可选择已存模型或手动填写上游模型 ID。');
    }).catch(e => { if (!abort.signal.aborted) setCatalogError(chatProblem(e) + ' 也可以手动填写模型 ID。'); })
      .finally(() => { if (!abort.signal.aborted) setCatalogLoading(false); });
    return () => abort.abort();
  }, [transport, providerId, workspace, keyId, catalogRevision]);
  return { providers, provider, providerId, setProviderId, workspace, keyId, setKeyId, model, setModel,
    models: workspace ? chatModels(workspace, keyId, catalog) : [], loading, catalogLoading, error, catalogError,
    refresh: () => setRevision(n => n + 1), refreshCatalog: () => setCatalogRevision(n => n + 1) };
}

function pause(signal: AbortSignal) {
  return new Promise<void>(resolve => {
    const done = () => { clearTimeout(timer); signal.removeEventListener('abort', done); resolve(); };
    const timer = setTimeout(done, 150); signal.addEventListener('abort', done, { once: true });
    if (signal.aborted) done();
  });
}
export function useChat(transport: ChatTransport) {
  const [turns, setTurns] = useState<ChatTurn[]>([]);
  const [draft, setDraft] = useState('');
  const [reasoningEffort, setReasoningEffort] = useState('');
  const [busy, setBusy] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState('');
  const [elapsed, setElapsed] = useState(0);
  const live = useRef(true);
  const locked = useRef(false);
  const stopRequested = useRef(false);
  const run = useRef<ChatRequest | null>(null);
  const request = useRef<AbortController | null>(null);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false; request.current?.abort();
      if (run.current) void transport.request({ ...run.current, action: 'cancel' }, new AbortController().signal).catch(() => {});
    };
  }, [transport]);
  function clear() { if (!locked.current) { setTurns([]); setDraft(''); setError(''); setElapsed(0); } }
  async function send(providerId: string, revision: number, keyId: string, model: string) {
    if (locked.current || !providerId || !keyId || !model.trim() || !draft.trim()) return;
    const content = draft.trim();
    const messages = chatMessages(turns, content); const invalid = messageProblem(messages);
    if (invalid) { setError(invalid); return; }
    locked.current = true; stopRequested.current = false; setBusy(true); setStopping(false); setError(''); setElapsed(0); setDraft('');
    const index = turns.length;
    setTurns(current => [...current, { content, reply: '', status: 'running' }]);
    const abort = new AbortController(); request.current = abort;
    const input: ChatRequest = { provider_id: providerId, expected_revision: revision, key_id: keyId, model: model.trim(), action: 'start', run_id: '', approved: true, messages, ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}) };
    const update = (reply: string, status: ChatTurn['status']) => setTurns(current => current.map((t, i) => i === index ? { ...t, reply, status } : t));
    let reply = '';
    try {
      // Keep the start response observable even if unmounted so its run can be cancelled.
      let next = await transport.request(input, new AbortController().signal);
      const current: ChatRequest = { ...input, action: 'read', run_id: next.run_id, messages: [] };
      run.current = current;
      if (!live.current || abort.signal.aborted) return;
      if (stopRequested.current) await transport.request({ ...current, action: 'cancel' }, abort.signal);
      while (true) {
        if (!live.current || abort.signal.aborted) return;
        reply = next.text; setElapsed(next.elapsed_ms);
        const failed = next.status === 'completed' && (!!next.result?.failure || !reply.trim());
        update(reply, next.status === 'running' ? 'running' : next.status === 'cancelled' ? 'cancelled' : failed ? 'failed' : 'completed');
        if (next.status !== 'running') { if (failed) setError(chatFailure(next.result?.failure || 'invalid_response')); break; }
        await pause(abort.signal); if (abort.signal.aborted) return;
        next = await transport.request(current, abort.signal);
      }
      run.current = null;
    } catch (e) {
      if (live.current && !abort.signal.aborted) { update(reply, 'failed'); setError(chatProblem(e)); }
    } finally {
      const current = run.current; run.current = null;
      if (current) void transport.request({ ...current, action: 'cancel' }, new AbortController().signal).catch(() => {});
      locked.current = false;
      if (live.current) { setBusy(false); setStopping(false); }
    }
  }
  async function stop() {
    stopRequested.current = true; setStopping(true);
    if (run.current) {
      try { await transport.request({ ...run.current, action: 'cancel' }, new AbortController().signal); }
      catch { if (live.current) { setStopping(false); setError('未能确认停止状态，生成将在超时后结束。'); } }
    }
  }
  return { turns, draft, setDraft, reasoningEffort, setReasoningEffort, busy, stopping, error, elapsed, clear, send, stop };
}
