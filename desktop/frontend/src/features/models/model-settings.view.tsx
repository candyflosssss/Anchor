import { useState } from 'react';
import type { ProviderTransport } from '../providers/provider.model';
import type { ModelGroup } from '../providers/provider-summary.model';
import { ModelSpeedSettingsDialog } from './model-speed-settings.view';
import type { ModelConfiguration, ReasoningOption } from '../../desktop/contract.generated';
import { Dialog, DialogContent } from '../../components/ui/dialog';
import { Button } from '../../components/ui/button';
import { useModelSettings } from './use-model-settings';
import { ContextPriceNotice, ContextPriceTable } from './model-context-settings.view';
import { crossesPriceBand, settingsReadDraft } from './model-settings.model';
import { ModelSettingsHeader } from './model-settings-header.view';
import './model-settings.css';

const triValue = (v: boolean | null) => v === null ? 'unknown' : v ? 'yes' : 'no';
const triParse = (v: string) => v === 'unknown' ? null : v === 'yes';
function Capability({ label, value, change }: { label: string; value: boolean | null; change: (v: boolean | null) => void }) {
  return <label className="model-setting-field"><span>{label}</span><select className="select-field" value={triValue(value)} onChange={e => change(triParse(e.target.value))}><option value="unknown">未知 / 未声明</option><option value="yes">支持</option><option value="no">不支持</option></select></label>;
}
function NumberField({ label, value, change, min = 1, step = 1 }: { label: string; value: number | null; change: (v: number | null) => void; min?: number; step?: number | 'any' }) {
  return <label className="model-setting-field"><span>{label}</span><input className="input-field" type="number" min={min} step={step} value={value ?? ''} placeholder="未设置" onChange={e => change(e.target.value === '' ? null : e.target.valueAsNumber)} /></label>;
}
function ReasoningFields({ cfg, change }: { cfg: ModelConfiguration; change: (cfg: ModelConfiguration) => void }) {
  const option = cfg.supported_reasoning[0], d = cfg.default_reasoning;
  const setOption = (o: ReasoningOption | null) => change({ ...cfg, capabilities: { ...cfg.capabilities, reasoning: o ? true : cfg.capabilities.reasoning }, supported_reasoning: o ? [o] : [], default_reasoning: null });
  const effortValues = [...new Set(['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'default', ...(option?.values.filter((v): v is string => v !== null) ?? [])])];
  return <div className="model-reasoning-fields">
    <Capability label="推理 / 思考能力" value={cfg.capabilities.reasoning} change={v => change({ ...cfg, capabilities: { ...cfg.capabilities, reasoning: v }, supported_reasoning: v === false ? [] : cfg.supported_reasoning, default_reasoning: v === false ? null : d })} />
    <label className="model-setting-field"><span>思考控制方式</span><select className="select-field" value={option?.type ?? ''} onChange={e => { const type = e.target.value; setOption(type ? { type, values: type === 'effort' ? ['medium'] : [], min: null, max: null } : null); }}><option value="">未声明范围</option><option value="effort">等级 effort</option><option value="toggle">开关 toggle</option><option value="budget_tokens">Token 预算</option></select></label>
    {option?.type === 'effort' && <><fieldset className="reasoning-values"><legend>支持的原生等级</legend>{[...effortValues, null].map(value => <label key={value ?? '__null'}><input type="checkbox" checked={option.values.includes(value)} onChange={e => setOption({ ...option, values: e.target.checked ? [...option.values, value] : option.values.filter(v => v !== value) })} />{value ?? 'JSON null'}</label>)}</fieldset>
      <label className="model-setting-field"><span>默认思考等级</span><select className="select-field" value={d?.type === 'effort' ? JSON.stringify(d.value) : ''} onChange={e => change({ ...cfg, default_reasoning: e.target.value ? { type: 'effort', value: JSON.parse(e.target.value) as string | null, budget_tokens: null, enabled: null } : null })}><option value="">沿用调用方默认值</option>{option.values.map(v => <option key={v ?? '__null'} value={JSON.stringify(v)}>{v ?? 'JSON null'}</option>)}</select></label></>}
    {option?.type === 'budget_tokens' && <div className="model-settings-grid"><NumberField label="最小思考预算（-1 为自动）" min={-1} value={option.min} change={v => change({ ...cfg, supported_reasoning: [{ ...option, min: v }] })} /><NumberField label="最大思考预算" min={0} value={option.max} change={v => change({ ...cfg, supported_reasoning: [{ ...option, max: v }] })} /><NumberField label="默认思考预算（-1 为自动）" min={-1} value={d?.budget_tokens ?? null} change={v => change({ ...cfg, default_reasoning: v === null ? null : { type: 'budget_tokens', value: null, enabled: null, budget_tokens: v } })} /></div>}
    {option?.type === 'toggle' && <label className="model-setting-field"><span>默认思考开关</span><select className="select-field" value={d?.enabled === undefined || d.enabled === null ? 'unknown' : triValue(d.enabled)} onChange={e => change({ ...cfg, default_reasoning: e.target.value === 'unknown' ? null : { type: 'toggle', value: null, budget_tokens: null, enabled: e.target.value === 'yes' } })}><option value="unknown">沿用调用方默认值</option><option value="yes">开启</option><option value="no">关闭</option></select></label>}
    {cfg.supported_reasoning.length > 1 && <p className="card-hint">目录包含多个原生控制方式；修改控制方式或支持范围将替换为当前选择的一种。完整配置可通过 CLI 保存。</p>}
  </div>;
}

type SettingsProps = { id: string; active: boolean; close: () => void; saved: () => Promise<void> };
export function ModelSettingsDialog({ group, transport, ...props }: SettingsProps & { group?: ModelGroup; transport?: ProviderTransport }) {
  const [page, setPage] = useState<'speed' | 'capabilities'>('capabilities');
  if (group && transport && page === 'speed') return <ModelSpeedSettingsDialog group={group} transport={transport} {...props} capabilities={() => setPage('capabilities')} />;
  return <ModelCapabilitiesDialog {...props} speed={group && transport ? () => setPage('speed') : undefined} />;
}
function ModelCapabilitiesDialog({ id, active, close, saved, speed }: SettingsProps & { speed?: () => void }) {
  const c = useModelSettings(id, active, saved), state = c.state, cfg = state?.configuration;
  const locked = !active || c.busy || c.needsRead;
  const setCap = (key: keyof ModelConfiguration['capabilities'], v: boolean | null | string[]) => {
    if (!cfg) return;
    const next = { ...cfg, capabilities: { ...cfg.capabilities, [key]: v } };
    if (key === 'vision' && typeof v === 'boolean' && next.capabilities.input_modalities.length) next.capabilities.input_modalities = v ? [...new Set([...next.capabilities.input_modalities, 'image'])] : next.capabilities.input_modalities.filter(x => x !== 'image');
    if (key === 'temperature' && v !== true) next.default_temperature = null;
    if (key === 'tool_call' && v === false) next.parallel_tool_calls = null;
    c.change(next);
  };
  const reference = state?.reference, sourceModel = reference?.models[0], pricing = sourceModel?.pricing;
  const defaultsNotice = state ? settingsReadDraft(state).notice : '';
  const operationNotice = c.notice !== defaultsNotice ? c.notice : '';
  const catalogStatus = ({ matched: c.unchanged ? '已补全' : '已补全 · 待保存', reference: '已关联', unmatched: '未匹配', unavailable: '暂不可用', incompatible: '需核对' } as Record<string, string>)[state?.defaults_status ?? ''];
  return <Dialog open={active} onOpenChange={open => { if (!open && !c.busy) close(); }}><DialogContent className="model-settings-dialog" onRequestClose={() => { if (!c.busy) close(); }}>
    <ModelSettingsHeader alias={id} page="general" busy={c.busy} switchPage={speed} />
    {c.error && <p className="feedback error-feedback model-settings-feedback" role="alert">{c.error}</p>}{operationNotice && <p className="feedback model-settings-feedback" role="status">{operationNotice}</p>}
    {(c.needsRead || (!state && !c.busy)) && <Button variant="outline" onClick={() => void c.read()} disabled={c.busy}>重新读取设置</Button>}
    {!state ? <p className="card-hint">{c.busy ? '正在读取模型设置…' : '未取得模型设置。'}</p> : cfg && <form onSubmit={e => { e.preventDefault(); if (!locked) void c.save(); }}>
      <div className="model-settings-body"><fieldset disabled={locked} className="model-settings-fields">
        <div className="model-settings-grid model-settings-basics">
        <label className="model-setting-field"><span>显示名称</span><input className="input-field" maxLength={256} value={state.display_name} onChange={e => c.displayName(e.target.value)} /></label>
        </div>
        <section className="settings-section"><h3>上下文与输出</h3><div className="model-settings-grid"><NumberField label="上下文窗口（tokens）" value={cfg.limits.context} change={v => c.change({ ...cfg, limits: { ...cfg.limits, context: v } })} /><NumberField label="最大输出（tokens）" value={cfg.limits.output} change={v => c.change({ ...cfg, limits: { ...cfg.limits, output: v } })} /></div><ContextPriceNotice state={state} acknowledged={c.longContextAcknowledged} acknowledge={c.setLongContextAcknowledged} change={c.change} /></section>
        <details className="settings-section"><summary>模型能力</summary><div className="model-settings-grid"><Capability label="视觉输入" value={cfg.capabilities.vision} change={v => setCap('vision', v)} /><Capability label="工具调用" value={cfg.capabilities.tool_call} change={v => setCap('tool_call', v)} /><Capability label="流式输出" value={cfg.streaming} change={v => c.change({ ...cfg, streaming: v })} /><Capability label="结构化输出" value={cfg.capabilities.structured_output} change={v => setCap('structured_output', v)} /></div></details>
        <details className="settings-section"><summary>思考设置</summary><ReasoningFields cfg={cfg} change={c.change} /></details>
        <details className="settings-section"><summary>高级设置</summary><div className="model-settings-grid"><NumberField label="最大输入（tokens）" value={cfg.limits.input} change={v => c.change({ ...cfg, limits: { ...cfg.limits, input: v } })} /><NumberField label="默认输出预算（tokens）" value={cfg.default_output_tokens} change={v => c.change({ ...cfg, default_output_tokens: v })} /><Capability label="附件输入" value={cfg.capabilities.attachment} change={v => setCap('attachment', v)} /><Capability label="Temperature 参数" value={cfg.capabilities.temperature} change={v => setCap('temperature', v)} /><NumberField label="默认 Temperature（0–2）" value={cfg.default_temperature} min={0} step="any" change={v => c.change({ ...cfg, default_temperature: v })} /><Capability label="并行工具调用" value={cfg.parallel_tool_calls} change={v => c.change({ ...cfg, parallel_tool_calls: v })} /></div>
        {(['input_modalities', 'output_modalities'] as const).map(key => <label key={key} className="model-setting-field"><span>{key === 'input_modalities' ? '输入模态' : '输出模态'}（逗号分隔）</span><input className="input-field" key={JSON.stringify(cfg.capabilities[key])} defaultValue={cfg.capabilities[key].join(', ')} onBlur={e => { const v = [...new Set(e.target.value.split(',').map(x => x.trim()).filter(Boolean))]; const capabilities = { ...cfg.capabilities, [key]: v }; if (key === 'input_modalities' && v.length) capabilities.vision = v.includes('image'); c.change({ ...cfg, capabilities }); }} /></label>)}</details>
        <details className="settings-section model-settings-catalog"><summary>目录与参考价格{catalogStatus && <span className="model-settings-summary-meta">{catalogStatus}</span>}</summary>{defaultsNotice && <p className="card-hint">{defaultsNotice}</p>}<Button type="button" variant="outline" size="sm" onClick={() => void c.loadProviders()}>读取 models.dev 供应商</Button><div className="model-settings-grid"><label className="model-setting-field"><span>目录供应商</span><select className="select-field" value={c.provider} onChange={e => void c.selectProvider(e.target.value)}><option value="">请选择</option>{c.provider && !c.providers.some(p => p.id === c.provider) && <option value={c.provider}>{c.provider}</option>}{c.providers.map(p => <option key={p.id} value={p.id}>{p.name} · {p.id}</option>)}</select></label><label className="model-setting-field"><span>精确模型 ID</span><input className="input-field" list={`catalog-models-${id}`} value={c.model} onChange={e => c.setModel(e.target.value)} placeholder="可选择或输入完整 ID" /><datalist id={`catalog-models-${id}`}>{c.models.map(m => <option key={m.id} value={m.id}>{m.name}</option>)}</datalist></label></div><div className="settings-actions"><Button type="button" size="sm" variant="outline" disabled={!c.provider || !c.model} onClick={() => void c.importModel(false)}>补充未设置的默认值</Button><Button type="button" size="sm" variant="outline" disabled={!c.provider || !c.model} onClick={() => void c.importModel(true)}>用目录值替换草稿</Button></div>
        {reference && sourceModel ? <div className="reference-snapshot"><p>models.dev / {sourceModel.provider_id} / {sourceModel.id}</p><p className="card-hint">抓取于 {new Date(reference.fetched_at * 1000).toLocaleString()}；模型更新：{sourceModel.source_updated_at || '未提供'}。版本 {reference.version.slice(0, 12)}。</p>{pricing ? <><p className="card-hint">参考价（USD / 百万 tokens），空项为未知；“0”为免费。实际费用以请求所用上游为准。</p><dl className="reference-prices">{Object.entries(pricing.rates).map(([key, value]) => <div key={key}><dt>{({ input: '输入', output: '输出', cache_read: '缓存读取', cache_write: '缓存写入', reasoning: '思考', input_audio: '音频输入', output_audio: '音频输出' } as Record<string, string>)[key]}</dt><dd>{value ?? '未知'}</dd></div>)}</dl><ContextPriceTable pricing={pricing} /></> : <p className="card-hint">目录没有提供价格。</p>}<Button type="button" size="sm" variant="outline" onClick={c.clearReference}>清除参考来源与快照</Button></div> : <p className="card-hint">未关联参考来源，可选择精确模型后导入。</p>}</details>
        <details className="model-settings-help"><summary>使用说明</summary>
          <p>Agent 配置需同步并重启。开关式／预算式思考、输出预算和 Temperature 仅保存为元数据，不自动改写请求。</p>
          {state.candidate_count > 1 && <p>此映射有 {state.candidate_count} 个上游候选，能力和限制应适用于所有候选；导入目录不会验证其他候选。</p>}
          <p>切换分类前请保存，未保存的修改会丢弃。</p>
        </details>
      </fieldset></div>
      <footer className="model-settings-footer"><Button type="button" variant="ghost" aria-label="重新读取并自动补全" disabled={locked} onClick={() => void c.read()}>重新读取</Button><div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={close}>关闭</Button><Button type="submit" disabled={locked || c.unchanged || (crossesPriceBand(state) && !c.longContextAcknowledged)}>{c.busy ? '处理中…' : '保存模型设置'}</Button></div></footer>
    </form>}
  </DialogContent></Dialog>;
}
