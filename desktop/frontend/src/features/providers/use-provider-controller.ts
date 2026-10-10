import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { isDirty, newDraft, problemText, ProviderProblem, toSave, validateDraft, type FieldErrors, type ProblemCode, type Provider, type ProviderDraft } from './provider.model';
import type { Scenario } from './provider.mock';
import type { ProviderEnvironment } from '../../desktop/environment';
type Phase = 'loading' | 'ready' | 'error';
export function useProviderController(environment: ProviderEnvironment) {
  const { transport, mock } = environment;
  const [providers, setProviders] = useState<Provider[]>([]);
  const [phase, setPhase] = useState<Phase>('loading');
  const [problem, setProblem] = useState<ProblemCode | null>(null);
  const [notice, setNotice] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [editor, setEditor] = useState<{ source?: Provider; draft: ProviderDraft } | null>(null);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [discard, setDiscard] = useState(false);
  const [busy, setBusy] = useState(false);
  const [toggle, setToggle] = useState<Provider | null>(null);
  const [scenario, setScenario] = useState<Scenario>('normal');
  // Single synchronous latch handles double clicks; Abort + generation reject stale reads.
  const locked = useRef(false);
  const query = useRef<AbortController | null>(null);
  const write = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const load = useCallback(async () => {
    query.current?.abort();
    const abort = new AbortController(); query.current = abort;
    const epoch = generation.current;
    setPhase('loading');
    try {
      const result = await transport.list(abort.signal);
      if (abort.signal.aborted || epoch !== generation.current) return;
      setProviders(result); setPhase('ready'); setProblem(null);
      setSelectedId(old => result.some(p => p.id === old) ? old : null);
    } catch (error) {
      if (abort.signal.aborted || epoch !== generation.current) return;
      setProviders([]); setPhase('error');
      setProblem(error instanceof ProviderProblem ? error.code : 'read_error');
    }
  }, [transport]);
  useEffect(() => {
    void load();
    return () => { query.current?.abort(); write.current?.abort(); };
  }, [load]);
  const visible = useMemo(() => providers.filter(p => (filter === 'all' || p.enabled === (filter === 'enabled')) && `${p.id} ${p.base_url}`.toLocaleLowerCase().includes(search.toLocaleLowerCase())), [providers, filter, search]);
  const selected = visible.find(p => p.id === selectedId) ?? null;
  const blocked = phase !== 'ready' || problem === 'unknown' || problem === 'conflict' || busy;
  function openEditor(source?: Provider) {
    if (blocked || locked.current) return;
    setNotice(''); setEditor({ source, draft: newDraft(source) }); setErrors({}); setDiscard(false);
  }
  function changeDraft<K extends keyof ProviderDraft>(key: K, value: ProviderDraft[K]) {
    setEditor(old => old && { ...old, draft: { ...old.draft, [key]: value,
      ...(key === 'secretMode' ? { secret: '', confirmClear: false } : {}) } });
    setDiscard(false);
    setErrors(old => { const next = { ...old }; delete next[key as keyof FieldErrors]; return next; });
  }
  function closeEditor(force = false) {
    if (locked.current) return;
    if (!force && editor && isDirty(editor.draft, editor.source)) { setDiscard(true); return; }
    setEditor(null); setErrors({}); setDiscard(false);
  }
  async function save(source: Provider | undefined, draft: ProviderDraft, toggling = false) {
    if (blocked || locked.current) return;
    const nextErrors = validateDraft(draft, !source);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    locked.current = true; setBusy(true); setNotice('');
    const abort = new AbortController(); write.current = abort;
    const epoch = generation.current;
    // Snapshot only for the transport call; erase the secret from the visible form immediately.
    const input = toSave(draft, source);
    setEditor(old => old && { ...old, draft: { ...old.draft, secret: '' } });
    try {
      const saved = await transport.save(input, abort.signal);
      if (abort.signal.aborted || epoch !== generation.current) return;
      setProviders(old => [...old.filter(p => p.id !== saved.id), saved]);
      setSelectedId(saved.id); setEditor(null); setToggle(null); setDiscard(false);
      setNotice(`${mock ? '演示：' : ''}${saved.id} 的配置已${toggling ? saved.enabled ? '启用' : '停用' : '保存'}（revision ${saved.revision}）。未测试上游连通性。`);
    } catch (error) {
      if (abort.signal.aborted || epoch !== generation.current) return;
      const code = error instanceof ProviderProblem ? error.code : 'unknown';
      setProblem(code); setToggle(null);
      // Conflict/unknown require an explicit read; never automatically replay a write.
      if (code === 'unknown') setEditor(null);
    } finally {
      input.secret = null;
      if (epoch === generation.current) { locked.current = false; setBusy(false); }
    }
  }
  async function reloadFromEditor() {
    // Deliberate, labeled discard action: user has already been told their draft is lost.
    closeEditor(true); await load();
    setNotice(`${mock ? '演示：' : ''}已重新读取。请核对最新配置，再打开编辑；原草稿已丢弃。`);
  }
  function changeScenario(value: Scenario) {
    if (!environment.reset || editor || busy || toggle) return;
    query.current?.abort(); write.current?.abort(); generation.current++;
    environment.reset(value); setScenario(value); setSelectedId(null); setSearch(''); setFilter('all'); setNotice(''); setProblem(null);
    void load();
  }
  return {
    mock, providers, visible, selected, selectedId, phase, problem, problemMessage: problem ? problemText[problem] : '',
    notice, search, setSearch, filter, setFilter, editor, errors, discard, busy, blocked, toggle, scenario,
    select: setSelectedId, load, openEditor, changeDraft, closeEditor, changeScenario, reloadFromEditor,
    continueEditing: () => setDiscard(false),
    submit: () => editor && void save(editor.source, editor.draft),
    requestToggle: (provider: Provider) => { if (!blocked) { setNotice(''); setToggle(provider); } },
    cancelToggle: () => { if (!locked.current) setToggle(null); },
    confirmToggle: () => { if (toggle) void save(toggle, { ...newDraft(toggle), enabled: !toggle.enabled }, true); },
  };
}
export type ProviderController = ReturnType<typeof useProviderController>;
