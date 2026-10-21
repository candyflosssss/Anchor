import { reconcileEntities } from '../../lib/keyed-refresh';
import { useProviderAccount } from './use-provider-account';
import { useProviderFeatures } from './use-provider-features';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { clearDraftSecrets, clearSaveSecrets, newDraft, problemText, ProviderProblem, toSave, validateDraft, type FieldErrors, type ProblemCode, type Provider, type ProviderDraft, type ProviderModels } from './provider.model';
import type { Scenario } from './provider.mock';
import type { DetailTab, ProviderDialog } from './provider-summary.model';
import type { ProviderEnvironment } from '../../desktop/environment';
import type { ProviderBalanceCache } from './provider-balance.model';
type Phase = 'loading' | 'ready' | 'error';
export function useProviderController(environment: ProviderEnvironment, observationCache: ProviderBalanceCache) {
  const { transport, mock } = environment;
  const [providers, setProviders] = useState<Provider[]>([]);
  const hasSnapshot = useRef(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [summaryVersion, setSummaryVersion] = useState(0);
  const [phase, setPhase] = useState<Phase>('loading');
  const [readVersion, setReadVersion] = useState(0);
  const [problem, setProblem] = useState<ProblemCode | null>(null);
  const [notice, setNotice] = useState('');
  const [quickId, setQuickId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [editor, setEditor] = useState<{ source?: Provider; draft: ProviderDraft; setup?: boolean; models?: boolean } | null>(null);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [busy, setBusy] = useState(false);
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
  const authorizedCatalog = useRef<{ id: string; revision: number } | null>(null);
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
  const load = useCallback(async (prefer?: string | null, scope: 'all' | 'provider' | 'models' = 'all') => {
    cancelCatalog();
    query.current?.abort();
    const abort = new AbortController(); query.current = abort;
    const epoch = generation.current;
    if (!hasSnapshot.current) setPhase('loading');
    setRefreshing(true);
    try {
      const result = await transport.list(abort.signal);
      if (abort.signal.aborted || epoch !== generation.current) return false;
      setProviders(old => reconcileEntities(old, result));
      setEditor(old => {
        if (!old?.models || !old.source) return old;
        const source = result.find(p => p.id === old.source!.id);
        return source ? { ...old, source, draft: newDraft(source) } : old;
      });
      hasSnapshot.current = true;
      if (scope === 'all') { setReadVersion(value => value + 1); setRefreshVersion(value => value + 1); }
      if (scope !== 'provider') setSummaryVersion(value => value + 1);
      setPhase('ready'); setProblem(null);
      setSelectedId(old => { const want = prefer !== undefined ? prefer : old; return result.some(p => p.id === want) ? want : null; });
      return true;
    } catch (error) {
      if (abort.signal.aborted || epoch !== generation.current) return false;
      if (!hasSnapshot.current) setPhase('error');
      setProblem(error instanceof ProviderProblem ? error.code : 'read_error');
      return false;
    } finally {
      if (query.current === abort) { query.current = null; setRefreshing(false); }
    }
  }, [transport, cancelCatalog]);
  useEffect(() => {
    void load();
    return () => { query.current?.abort(); write.current?.abort(); catalogQuery.current?.abort(); };
  }, [load]);
  const visible = useMemo(() => providers.filter(p => (filter === 'all' || p.enabled === (filter === 'enabled')) && `${p.id} ${p.homepage_url} ${p.base_urls.join(' ')}`.toLocaleLowerCase().includes(search.toLocaleLowerCase())), [providers, filter, search]);
  const selected = providers.find(p => p.id === (quickId ?? selectedId)) ?? null;
  const blocked = phase !== 'ready' || refreshing || (!!problem && problem !== 'invalid' && problem !== 'referenced') || busy || modelsBusy;
  function openEditor(source?: Provider) {
    if (blocked || locked.current || (features.isBusy() || account.isBusy())) return;
    setNotice(''); setEditor({ source, draft: newDraft(source) }); setErrors({});
  }
  function changeDraft<K extends keyof ProviderDraft>(key: K, value: ProviderDraft[K]) {
    setEditor(old => old && { ...old, draft: { ...old.draft, [key]: value,
      ...(key === 'secretMode' ? { secret: '', confirmClear: false } : {}) } });
    setErrors(old => {
      const next = { ...old }; delete next[key as keyof FieldErrors];
      if (key === 'initialKeys') for (const name of Object.keys(next)) if (name.startsWith('initial_key_')) delete next[name as keyof FieldErrors];
      if (key === 'base_urls') for (const name of Object.keys(next)) if (name.startsWith('base_url_')) delete next[name as keyof FieldErrors];
      return next;
    });
  }
  function closeEditor() {
    if (locked.current || features.isBusy() || account.isBusy()) return;
    // Cancel / close / Escape always discard the draft directly, without a second prompt.
    setEditor(null); setErrors({});
  }
  async function save(source: Provider | undefined, draft: ProviderDraft, toggling = false) {
    if (blocked || locked.current || (features.isBusy() || account.isBusy())) return;
    const nextErrors = validateDraft(draft);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    if (!toggling) clearModels();
    locked.current = true; setBusy(true);
    const abort = new AbortController(); write.current = abort;
    const epoch = generation.current;
    // Snapshot only for the transport call; erase the secret from the visible form immediately.
    const input = toSave(draft, source);
    const setup = !toggling && (editor?.setup || (!source && (!!input.secret || !!input.initial_keys?.length)));
    setEditor(old => old && { ...old, draft: clearDraftSecrets(old.draft) });
    try {
      const saved = await transport.save(input, abort.signal);
      clearSaveSecrets(input);
      if (abort.signal.aborted || epoch !== generation.current) return;
      if (!toggling) clearModels();
      if (toggling && source) observationCache.retainToggle(source, saved);
      // Update existing rows in place so saving or toggling never changes their position.
      setProviders(old => old.some(p => p.id === saved.id) ? old.map(p => p.id === saved.id ? saved : p) : [...old, saved]);
      if (!source) { setSelectedId(saved.id); setDetailTab('overview'); setDialog(null); }
      setEditor(setup ? { source: saved, draft: newDraft(saved), setup: true, models: true } : null);
      // Reconcile newly created providers with the authoritative list automatically.
      if (!source) await load(saved.id, 'provider');
      if (abort.signal.aborted || epoch !== generation.current) return;
      if (setup) {
        setDetailTab('models');
        await fetchModels(saved);
        return;
      }
      setNotice(`${mock ? '演示：' : ''}${saved.id} 的配置已${toggling ? saved.enabled ? '启用' : '停用' : '保存'}（revision ${saved.revision}）。未测试上游连通性。`);
    } catch (error) {
      if (abort.signal.aborted || epoch !== generation.current) return;
      const code = error instanceof ProviderProblem ? error.code : 'unknown';
      setProblem(code);
      // Conflict/unknown require an explicit read; never automatically replay a write.
      if (code === 'unknown') setEditor(null);
    } finally {
      clearSaveSecrets(input);
      if (epoch === generation.current) { locked.current = false; setBusy(false); }
    }
  }
  async function discoverModels() {
    if (!selected || blocked || locked.current || (features.isBusy() || account.isBusy()) || (editor && !editor.models)) return;
    await fetchModels(selected);
  }
  async function fetchModels(provider: Provider) {
    const target = { provider_id: provider.id, expected_revision: provider.revision, key_id: '' };
    cancelCatalog(); catalogEpoch.current++;
    setModels(old => old?.provider_id === provider.id && old.revision === provider.revision ? old : null);
    setModelsProblem(null); locked.current = true; setModelsBusy(true);
    const abort = new AbortController(); catalogQuery.current = abort;
    const epoch = catalogEpoch.current;
    try {
      const result = await transport.discover(target, abort.signal);
      if (abort.signal.aborted || epoch !== catalogEpoch.current) return;
      setModels(result);
      await features.refresh();
    } catch (error) {
      if (abort.signal.aborted || epoch !== catalogEpoch.current) return;
      const code = error instanceof ProviderProblem ? error.code : 'read_error';
      // This operation never writes; never show the "possibly saved" message.
      setModelsProblem(code === 'unknown' ? 'read_error' : code);
    } finally {
      if (epoch === catalogEpoch.current) { catalogQuery.current = null; locked.current = false; setModelsBusy(false); }
    }
  }
  function editSetup() {
    if (!selected || locked.current || features.isBusy() || account.isBusy()) return;
    setProblem(null); setErrors({});
    setEditor({ source: selected, draft: newDraft(selected), setup: true });
  }
  async function reloadFromEditor() {
    // Deliberate, labeled discard action: user has already been told their draft is lost.
    closeEditor(); await load();
    setNotice(`${mock ? '演示：' : ''}已重新读取。请核对最新配置，再打开编辑；原草稿已丢弃。`);
  }
  function changeScenario(value: Scenario) {
    if (!environment.reset || editor || busy || modelsBusy || (features.isBusy() || account.isBusy())) return;
    clearModels(); query.current?.abort(); write.current?.abort(); generation.current++;
    hasSnapshot.current = false; setProviders([]);
    environment.reset(value); setScenario(value); setQuickId(null); setSelectedId(null); setDetailTab('overview'); setDialog(null); setSearch(''); setFilter('all'); setNotice(''); setProblem(null);
    void load();
  }
  const features = useProviderFeatures(selected, transport, async (id, action, from) => {
    if (['key-put', 'key-delete', 'rename', 'copy', 'delete'].includes(action)) clearModels();
    // Shared model revisions can change without changing every provider revision.
    const sharedModels = ['model-put', 'model-delete', 'key-delete', 'rename', 'copy', 'delete'].includes(action);
    const scope = action === 'conflict' ? 'all' : sharedModels ? 'models' : 'provider';
    if (action === 'rename' || action === 'copy' || action === 'delete') {
      await load(id, scope);
      setDetailTab('overview'); setDialog(null);
      setNotice(action === 'rename' ? `已改名为 ${id}` : action === 'copy' ? `已复制为 ${id}（默认停用）` : `已删除 ${from}`);
    } else await load(undefined, scope);
  }, () => setNotice(''), readVersion);
  const account = useProviderAccount(selected, transport, async saved => {
    if (models || editor?.models) authorizedCatalog.current = { id: saved.provider_id, revision: saved.revision };
    clearModels();
    await load(undefined, 'provider');
    setNotice(`${mock ? '演示：' : ''}平台配置已保存。`);
  }, () => blocked || locked.current || features.busy || (!!editor && !editor.models) || !!dialog, readVersion,
  !blocked && !features.busy && (!editor || !!editor.models) && !dialog && !!features.data?.keys.some(k => k.enabled),
  () => features.refresh(), selected ? observationCache.queryRevision(selected) : undefined);
  const catalogAfterAuth = useRef(() => {});
  catalogAfterAuth.current = () => {
    const pending = authorizedCatalog.current;
    if (!pending) return;
    if (pending.id !== selected?.id) { authorizedCatalog.current = null; return; }
    if (pending.revision !== selected.revision || blocked || features.busy || locked.current || account.isBusy()) return;
    authorizedCatalog.current = null;
    void fetchModels(selected);
  };
  useEffect(() => { catalogAfterAuth.current(); }, [selected, blocked, features.busy, account.busy]);
  function openProvider(id: string, tab: DetailTab = 'overview', next?: ProviderDialog) {
    if (locked.current || (features.isBusy() || account.isBusy()) || dialog || editor) return;
    if (id !== selectedId) { clearModels(); setNotice(''); }
    setQuickId(null); setSelectedId(id); setDetailTab(tab); setDialog(next ?? null);
  }
  return {
    account,
    features: { ...features, importModels: (ids: string[]) => account.isBusy() || locked.current || blocked ? Promise.resolve() : features.importModels(ids, Object.fromEntries((models?.entries ?? []).map(e => [e.id, e.key_ids]))), run: (...args: Parameters<typeof features.run>) => account.isBusy() ? Promise.resolve(false) : features.run(...args) },
    transport, quickId, openQuickTest: (id: string) => { if (!blocked && !locked.current && !dialog && !features.isBusy() && !account.isBusy()) { setQuickId(id); setDialog({ kind: 'test' }); } },
    mock, providers, visible, selected, selectedId, phase, refreshing, refreshVersion, summaryVersion, problem, problemMessage: problem ? problemText[problem] : '',
    notice, search, setSearch, filter, setFilter, editor, errors, busy: refreshing || busy || modelsBusy || features.busy || account.busy, blocked: blocked || features.busy || account.busy, scenario,
    models: models && selected && models.provider_id === selected.id && models.revision === selected.revision ? models : null,
    modelsProblem, modelsBusy, discoverModels, editSetup,
    detailTab, setDetailTab, dialog, openProvider,
    openDialog: (next: ProviderDialog) => { if (!(features.isBusy() || account.isBusy()) && !locked.current) setDialog(next); },
    closeDialog: () => { if (!(features.isBusy() || account.isBusy())) { setDialog(null); setQuickId(null); } },
    select: (id: string | null) => { if (!locked.current && !(features.isBusy() || account.isBusy())) { clearModels(); setQuickId(null); setSelectedId(id); setDetailTab('overview'); setDialog(null); } }, load, openEditor, changeDraft, closeEditor, changeScenario, reloadFromEditor,
    submit: () => editor && void save(editor.source, editor.draft),
    toggleProvider: (provider: Provider) => { if (!editor && !dialog) void save(provider, { ...newDraft(provider), enabled: !provider.enabled }, true); },
  };
}
export type ProviderController = ReturnType<typeof useProviderController>;
