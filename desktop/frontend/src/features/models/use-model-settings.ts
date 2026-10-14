import { useEffect, useRef, useState } from 'react';
import type { ModelCatalog, ModelConfiguration, ModelSettingsState, SaveModelSettings } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { modelCatalogService } from '../model-catalog/model-catalog.model';
import { importConfiguration, modelSettingsService, settingsError } from './model-settings.model';

export function useModelSettings(id: string, active: boolean, saved: () => Promise<void>) {
  const [state, setState] = useState<ModelSettingsState | null>(null), [error, setError] = useState(''), [notice, setNotice] = useState(''), [busy, setBusy] = useState(false), [needsRead, setNeedsRead] = useState(false);
  const [providers, setProviders] = useState<ModelCatalog['providers']>([]), [models, setModels] = useState<ModelCatalog['models']>([]), [provider, setProvider] = useState(''), [model, setModel] = useState('');
  const [referenceAction, setReferenceAction] = useState<SaveModelSettings['reference_action']>('keep');
  const referenceID = useRef({ provider: '', model: '' });
  const epoch = useRef(0), lock = useRef(false), abort = useRef<AbortController | null>(null);
  async function run(work: (signal: AbortSignal) => Promise<void>) {
    if (!active || lock.current) return;
    lock.current = true; setBusy(true); setError(''); const at = epoch.current, request = new AbortController(); abort.current = request;
    try { await work(request.signal); } catch (e) { if (!request.signal.aborted && at === epoch.current) { setError(settingsError(e)); if (e instanceof DesktopProblem && ['conflict', 'unknown'].includes(e.code)) setNeedsRead(true); } }
    finally { if (at === epoch.current) { lock.current = false; setBusy(false); } }
  }
  async function read() { await run(async signal => { const v = await modelSettingsService.read(id, signal); setState(v); setNeedsRead(false); setReferenceAction('keep'); setNotice(''); }); }
  useEffect(() => {
    const lifecycle = { epoch, abort, lock };
    lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false;
    setState(null); setError(''); setNotice(''); setBusy(false); setNeedsRead(false); setProviders([]); setModels([]); setProvider(''); setModel(''); setReferenceAction('keep');
    if (active) {
      const request = new AbortController(); lifecycle.abort.current = request; lifecycle.lock.current = true; setBusy(true);
      void modelSettingsService.read(id, request.signal).then(v => { if (!request.signal.aborted) setState(v); }).catch(e => { if (!request.signal.aborted) setError(settingsError(e)); }).finally(() => { if (!request.signal.aborted) { lifecycle.lock.current = false; setBusy(false); } });
    }
    return () => { lifecycle.epoch.current++; lifecycle.abort.current?.abort(); lifecycle.lock.current = false; };
  }, [id, active]);
  function change(configuration: ModelConfiguration) { if (active && !lock.current && !needsRead) { setState(v => v && ({ ...v, configuration })); setNotice(''); } }
  function displayName(value: string) { if (!lock.current && !needsRead) setState(v => v && ({ ...v, display_name: value })); }
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
      setState({ ...state, configuration: importConfiguration(state.configuration, v, replace), reference: v });
      referenceID.current = { provider, model }; setReferenceAction('set'); setNotice('已导入草稿，请核对所有上游候选后保存。');
    });
  }
  function clearReference() { if (!lock.current && !needsRead) { setState(v => v && ({ ...v, reference: null })); setReferenceAction('clear'); } }
  async function save() {
    if (!state || needsRead) return;
    await run(async signal => {
      const v = await modelSettingsService.save({ model_id: id, expected_revision: state.revision, display_name: state.display_name, configuration: state.configuration, reference_action: referenceAction, reference_provider_id: referenceAction === 'set' ? referenceID.current.provider : '', reference_model_id: referenceAction === 'set' ? referenceID.current.model : '' }, signal);
      setState(v); setReferenceAction('keep'); setNotice('模型设置已保存。需要更新 Agent 配置时，请执行同步并重启 Agent。'); await saved();
    });
  }
  return { state, error, notice, busy, needsRead, providers, models, provider, model, change, displayName, read, loadProviders, selectProvider, setModel, importModel, clearReference, save };
}
