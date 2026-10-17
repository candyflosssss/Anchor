import { useCallback, useEffect, useRef, useState } from 'react';
import type { ModelCatalog, ModelConfiguration, ModelSettingsState, SaveModelSettings } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { modelCatalogService } from '../model-catalog/model-catalog.model';
import { crossesPriceBand, importConfiguration, modelSettingsService, parseChannelRetries, settingsError, settingsReadDraft } from './model-settings.model';

export function useModelSettings(id: string, active: boolean, saved: () => Promise<void>) {
  const [state, setState] = useState<ModelSettingsState | null>(null), [error, setError] = useState(''), [notice, setNotice] = useState(''), [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false);
  const [providers, setProviders] = useState<ModelCatalog['providers']>([]), [models, setModels] = useState<ModelCatalog['models']>([]), [provider, setProvider] = useState(''), [model, setModel] = useState('');
  const [referenceAction, setReferenceAction] = useState<SaveModelSettings['reference_action']>('keep');
  const [retryCount, setRetryCount] = useState('3');
  const retryValidation = parseChannelRetries(retryCount) === null ? '重试次数须为 0–10 的整数。' : '';
  const [longContextAcknowledged, setLongContextAcknowledged] = useState(false);
  const savedDraft = useRef(false);
  const [unchanged, setUnchanged] = useState(false);
  const referenceID = useRef({ provider: '', model: '' });
  const epoch = useRef(0), lock = useRef(false), abort = useRef<AbortController | null>(null);
  async function run(work: (signal: AbortSignal) => Promise<void>) {
    if (!active || lock.current) return;
    lock.current = true; setBusy(true); setError(''); const at = epoch.current, request = new AbortController(); abort.current = request;
    try { await work(request.signal); } catch (e) { if (!request.signal.aborted && at === epoch.current) { setError(settingsError(e)); if (e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true); } }
    finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  const acceptRead = useCallback((value: ModelSettingsState) => {
    savedDraft.current = false; setUnchanged(false);
    const draft = settingsReadDraft(value);
    setRetryCount(String(value.channel_retries)); setLongContextAcknowledged(false); setState(value); setNeedsRead(false); setReferenceAction(draft.referenceAction); setNotice(draft.notice);
    setProvider(draft.provider); setModel(draft.model); setModels([]);
    referenceID.current = { provider: draft.provider, model: draft.model };
  }, []);
  async function read() { await run(async signal => { acceptRead(await modelSettingsService.read(id, signal)); }); }
  useEffect(() => {
    const lifecycle = { epoch, abort, lock };
    lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false;
    savedDraft.current = false; setUnchanged(false); setRetryCount('3'); setState(null); setLongContextAcknowledged(false); setError(''); setNotice(''); setBusy(false); setNeedsRead(false); setProviders([]); setModels([]); setProvider(''); setModel(''); setReferenceAction('keep');
    if (active) {
      const request = new AbortController(); lifecycle.abort.current = request; lifecycle.lock.current = true; setBusy(true);
      void modelSettingsService.read(id, request.signal).then(v => { if (!request.signal.aborted) { acceptRead(v); } }).catch(e => { if (!request.signal.aborted) setError(settingsError(e)); }).finally(() => { if (!request.signal.aborted) { lifecycle.lock.current = false; setBusy(false); } });
    }
    return () => { lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false; };
  }, [id, active, acceptRead]);
  function change(configuration: ModelConfiguration) { if (active && !lock.current && !needsRead) { savedDraft.current = false; setUnchanged(false); setLongContextAcknowledged(false); setState(v => v && ({ ...v, configuration })); setNotice(''); } }
  function changeRetries(value: string) { if (active && !lock.current && !needsRead) { savedDraft.current = false; setUnchanged(false); setRetryCount(value); setNotice(''); } }
  function displayName(value: string) { if (!lock.current && !needsRead) { savedDraft.current = false; setUnchanged(false); setState(v => v && ({ ...v, display_name: value })); } }
  async function loadProviders() { await run(async signal => { const v = await modelCatalogService.query({ provider_id: '', model_id: '', search: '', limit: 0, refresh: false }, signal); setProviders(v.providers); }); }
  async function selectProvider(value: string) {
    if (lock.current) return; setProvider(value); setModel(''); setModels([]);
    if (value) await run(async signal => { const v = await modelCatalogService.query({ provider_id: value, model_id: '', search: '', limit: 100, refresh: false }, signal); setModels(v.models); if (v.truncated) setNotice('目录超过 100 个模型，可直接输入精确模型 ID。'); });
  }
  async function importModel(replace: boolean) {
    if (!state || needsRead || !provider || !model) return;
    await run(async signal => {
      const v = await modelCatalogService.query({ provider_id: provider, model_id: model, search: '', limit: 0, refresh: true }, signal);
      if (v.stale) throw new DesktopProblem('read_error');
      setLongContextAcknowledged(false);
      savedDraft.current = false; setUnchanged(false);
      setState({ ...state, configuration: importConfiguration(state.configuration, v, replace), reference: v, context_policy: v.models[0].context_policy });
      referenceID.current = { provider, model }; setReferenceAction('set'); setNotice('已将上下文、能力和参考价格填入草稿；未提供的字段保持未知。请核对所有上游候选并点击「保存模型设置」。参考价格不会覆盖供应商计费单价。');
    });
  }
  function clearReference() { if (!lock.current && !needsRead) { savedDraft.current = false; setUnchanged(false); setLongContextAcknowledged(false); setState(v => v && ({ ...v, reference: null })); setReferenceAction('clear'); } }
  async function save() {
    if (!state || needsRead || savedDraft.current) return;
    const channelRetries = parseChannelRetries(retryCount);
    if (channelRetries === null) { setError('重试次数须为 0–10 的整数。'); return; }
    if (crossesPriceBand(state) && !longContextAcknowledged) { setError('当前窗口允许跨入另一上下文计费档，请先确认费用提示或使用基础价参考窗口。'); return; }
    await run(async signal => {
      const v = await modelSettingsService.save({ channel_retries: channelRetries, model_id: id, expected_revision: state.revision, display_name: state.display_name, configuration: state.configuration, reference_action: referenceAction, reference_provider_id: referenceAction === 'set' ? referenceID.current.provider : '', reference_model_id: referenceAction === 'set' ? referenceID.current.model : '' }, signal);
      savedDraft.current = true; setUnchanged(true); setRetryCount(String(v.channel_retries)); setState({ ...v, context_policy: v.context_policy ?? state.context_policy }); setLongContextAcknowledged(false); setReferenceAction('keep'); setNotice('模型设置已保存，重试次数对后续请求立即生效。若还修改了 Agent 能力配置，请同步并重启 Agent。'); await saved();
    });
  }
  return { unchanged, retryCount, changeRetries, retryValidation, longContextAcknowledged, setLongContextAcknowledged, state, error, notice, busy, needsRead, providers, models, provider, model, change, displayName, read, loadProviders, selectProvider, setModel, importModel, clearReference, save };
}
