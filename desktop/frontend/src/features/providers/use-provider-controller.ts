import { useProviderAccount } from './use-provider-account';
import { useProviderFeatures } from './use-provider-features';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { newDraft, problemText, ProviderProblem, toSave, validateDraft, type FieldErrors, type ProblemCode, type Provider, type ProviderDraft, type ProviderModels } from './provider.model';
import type { Scenario } from './provider.mock';
import type { DetailTab, ProviderDialog } from './provider-summary.model';
import type { ProviderEnvironment } from '../../desktop/environment';
type Phase = 'loading' | 'ready' | 'error';
export function useProviderController(environment: ProviderEnvironment) {
  const { transport, mock } = environment;
  const [providers, setProviders] = useState<Provider[]>([]);
  const [phase, setPhase] = useState<Phase>('loading');
  const [readVersion, setReadVersion] = useState(0);
  const [problem, setProblem] = useState<ProblemCode | null>(null);
  const [notice, setNotice] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [editor, setEditor] = useState<{ source?: Provider; draft: ProviderDraft } | null>(null);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [busy, setBusy] = useState(false);
  const [toggle, setToggle] = useState<Provider | null>(null);
  const [scenario, setScenario] = useState<Scenario>('normal');
  const [detailTab, setDetailTab] = useState<DetailTab>('overview');
  const [dialog, setDialog] = useState<ProviderDialog | null>(null);
  // Single synchronous latch handles double clicks; Abort + generation reject stale reads.
  const locked = useRef(false);
  const query = useRef<AbortController | null>(null);
  const write = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const [models, setModels] = useState<ProviderModels | null>(null);
  const [modelsProblem, setModelsProblem] = useState<ProblemCode | null>(null);
  const [modelsBusy, setModelsBusy] = useState(false);
  const catalogQuery = useRef<AbortController | null>(null);
  const catalogEpoch = useRef(0);
  const clearModels = useCallback(() => {
    catalogEpoch.current++;
    if (catalogQuery.current) { catalogQuery.current.abort(); catalogQuery.current = null; locked.current = false; }
    setModels(null); setModelsProblem(null); setModelsBusy(false);
  }, []);
  // Reloading the list must not throw away catalog results the user just fetched.
  const cancelCatalog = useCallback(() => {
    if (catalogQuery.current) { catalogEpoch.current++; catalogQuery.current.abort(); catalogQuery.current = null; locked.current = false; setModelsBusy(false); }
  }, []);
  /** prefer: undefined keeps the current selection, null clears it, a string selects that id when present. */
  const load = useCallback(async (prefer?: string | null) => {
    cancelCatalog();
    query.current?.abort();
    const abort = new AbortController(); query.current = abort;
    const epoch = generation.current;
    setPhase('loading');
    try {
      const result = await transport.list(abort.signal);
      if (abort.signal.aborted || epoch !== generation.current) return;
      setProviders(result); setReadVersion(value => value + 1); setPhase('ready'); setProblem(null);
      setSelectedId(old => { const want = prefer !== undefined ? prefer : old; return result.some(p => p.id === want) ? want : null; });
    } catch (error) {
      if (abort.signal.aborted || epoch !== generation.current) return;
      setProviders([]); setPhase('error');
      setProblem(error instanceof ProviderProblem ? error.code : 'read_error');
    }
  }, [transport, cancelCatalog]);
  useEffect(() => {
    void load();
    return () => { query.current?.abort(); write.current?.abort(); catalogQuery.current?.abort(); };
  }, [load]);
  const visible = useMemo(() => providers.filter(p => (filter === 'all' || p.enabled === (filter === 'enabled')) && `${p.id} ${p.homepage_url} ${p.base_urls.join(' ')}`.toLocaleLowerCase().includes(search.toLocaleLowerCase())), [providers, filter, search]);
  const selected = providers.find(p => p.id === selectedId) ?? null;
  const blocked = phase !== 'ready' || problem === 'unknown' || problem === 'conflict' || busy || modelsBusy;
  function openEditor(source?: Provider) {
    if (blocked || locked.current || (features.isBusy() || account.isBusy())) return;
    setNotice(''); setEditor({ source, draft: newDraft(source) }); setErrors({});
  }
  function changeDraft<K extends keyof ProviderDraft>(key: K, value: ProviderDraft[K]) {
    setEditor(old => old && { ...old, draft: { ...old.draft, [key]: value,
      ...(key === 'secretMode' ? { secret: '', confirmClear: false } : {}) } });
    setErrors(old => {
      const next = { ...old }; delete next[key as keyof FieldErrors];
      if (key === 'base_urls') for (const name of Object.keys(next)) if (name.startsWith('base_url_')) delete next[name as keyof FieldErrors];
      return next;
    });
  }
  function closeEditor() {
    if (locked.current) return;
    // Cancel / close / Escape always discard the draft directly, without a second prompt.
    setEditor(null); setErrors({});
  }
  async function save(source: Provider | undefined, draft: ProviderDraft, toggling = false) {
    if (blocked || locked.current || (features.isBusy() || account.isBusy())) return;
    const nextErrors = validateDraft(draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    clearModels();
    locked.current = true; setBusy(true); setNotice('');
    const abort = new AbortController(); write.current = abort;
    const epoch = generation.current;
    // Snapshot only for the transport call; erase the secret from the visible form immediately.
    const input = toSave(draft, source);
    setEditor(old => old && { ...old, draft: { ...old.draft, secret: '' } });
    try {
      const saved = await transport.save(input, abort.signal);
      if (abort.signal.aborted || epoch !== generation.current) return;
      clearModels();
      setProviders(old => [...old.filter(p => p.id !== saved.id), saved]);
      if (!source) { setSelectedId(saved.id); setDetailTab('overview'); setDialog(null); }
      setEditor(null); setToggle(null);
      // Reconcile newly created providers with the authoritative list automatically.
      if (!source) await load(saved.id);
      if (abort.signal.aborted || epoch !== generation.current) return;
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
  async function discoverModels() {
    if (!selected || blocked || locked.current || (features.isBusy() || account.isBusy()) || editor || toggle) return;
    const target = { provider_id: selected.id, expected_revision: selected.revision, key_id: '' };
    clearModels(); locked.current = true; setModelsBusy(true);
    const abort = new AbortController(); catalogQuery.current = abort;
    const epoch = catalogEpoch.current;
    try {
      const result = await transport.discover(target, abort.signal);
      if (abort.signal.aborted || epoch !== catalogEpoch.current) return;
      setModels(result);
    } catch (error) {
      if (abort.signal.aborted || epoch !== catalogEpoch.current) return;
      const code = error instanceof ProviderProblem ? error.code : 'read_error';
      // This operation never writes; never show the "possibly saved" message.
      setModelsProblem(code === 'unknown' ? 'read_error' : code);
    } finally {
      if (epoch === catalogEpoch.current) { catalogQuery.current = null; locked.current = false; setModelsBusy(false); }
    }
  }
  async function reloadFromEditor() {
    // Deliberate, labeled discard action: user has already been told their draft is lost.
    closeEditor(); await load();
    setNotice(`${mock ? '演示：' : ''}已重新读取。请核对最新配置，再打开编辑；原草稿已丢弃。`);
  }
  function changeScenario(value: Scenario) {
    if (!environment.reset || editor || busy || modelsBusy || (features.isBusy() || account.isBusy()) || toggle) return;
    clearModels(); query.current?.abort(); write.current?.abort(); generation.current++;
    environment.reset(value); setScenario(value); setSelectedId(null); setDetailTab('overview'); setDialog(null); setSearch(''); setFilter('all'); setNotice(''); setProblem(null);
    void load();
  }
  const features = useProviderFeatures(selected, transport, async (id, action, from) => {
    if (['key-put', 'key-delete', 'rename', 'copy', 'delete'].includes(action)) clearModels();
    if (action === 'rename' || action === 'copy' || action === 'delete') {
      await load(id);
      setDetailTab('overview'); setDialog(null);
      setNotice(action === 'rename' ? `已改名为 ${id}` : action === 'copy' ? `已复制为 ${id}（默认停用）` : `已删除 ${from}`);
    } else await load();
  }, () => setNotice(''), readVersion);
  const account = useProviderAccount(selected, transport, async () => {
    clearModels(); await load();
    setNotice(`${mock ? '演示：' : ''}平台配置已保存。可在「账户与余额」刷新余额或 Key 额度。`);
  }, () => blocked || locked.current || features.busy || !!editor || !!toggle || !!dialog, readVersion);
  function openProvider(id: string, tab: DetailTab = 'overview', next?: ProviderDialog) {
    if (locked.current || (features.isBusy() || account.isBusy()) || dialog || editor || toggle) return;
    if (id !== selectedId) { clearModels(); setNotice(''); }
    setSelectedId(id); setDetailTab(tab); setDialog(next ?? null);
  }
  return {
    account,
    features: { ...features, run: (...args: Parameters<typeof features.run>) => account.isBusy() ? Promise.resolve(false) : features.run(...args) },
    mock, providers, visible, selected, selectedId, phase, problem, problemMessage: problem ? problemText[problem] : '',
    notice, search, setSearch, filter, setFilter, editor, errors, busy: busy || modelsBusy || features.busy || account.busy, blocked: blocked || features.busy || account.busy, toggle, scenario,
    models: models?.provider_id === selected?.id ? models : null,
    modelsProblem, modelsBusy, discoverModels,
    detailTab, setDetailTab, dialog, openProvider,
    openDialog: (next: ProviderDialog) => { if (!(features.isBusy() || account.isBusy()) && !locked.current) setDialog(next); },
    closeDialog: () => { if (!(features.isBusy() || account.isBusy())) setDialog(null); },
    select: (id: string | null) => { if (!locked.current && !(features.isBusy() || account.isBusy())) { clearModels(); setSelectedId(id); setDetailTab('overview'); setDialog(null); } }, load, openEditor, changeDraft, closeEditor, changeScenario, reloadFromEditor,
    submit: () => editor && void save(editor.source, editor.draft),
    requestToggle: (provider: Provider) => { if (!blocked && !(features.isBusy() || account.isBusy())) { setNotice(''); setToggle(provider); } },
    cancelToggle: () => { if (!locked.current) setToggle(null); },
    confirmToggle: () => { if (toggle) void save(toggle, { ...newDraft(toggle), enabled: !toggle.enabled }, true); },
  };
}
export type ProviderController = ReturnType<typeof useProviderController>;
