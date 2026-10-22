import type { ProviderTransport } from '../providers/provider.model';
import { useState, type ReactNode } from 'react';
import { ArrowLeftRight, ArrowUp, ArrowDown, Check, ChevronDown, GripVertical, LoaderCircle, RefreshCw, Settings2, Undo2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { aggregateModels, type DetailTab, type ModelCandidate, type ModelGroup, type ProviderSummary } from '../providers/provider-summary.model';
import { channelKeys, keyAvailability, type SourceKey } from '../providers/channel-availability.model';
import { KeyMultiplierView } from '../providers/key-multiplier.view';
import { rateFailureText } from '../providers/provider-account.model';
import { rateID, type MappingRatesController, type RateTarget } from './use-mapping-rates';
import { modelAvailability, visibleModels } from './model-availability.model';
import { effectiveRouteMultiplier, routingRules, routeID } from './model-routing.model';
import type { ModelRoutingController } from './use-model-routing';
import { useRouteDrag } from './use-route-drag';
import { ModelSettingsDialog } from './model-settings.view';
import './models.css';
const candidateID = (m: ModelCandidate) => routeID({ provider_id: m.provider_id, model: m.upstream });
const sourceTargets = (m: ModelCandidate): RateTarget[] => channelKeys(m.key_ids, m.keys).filter(k => !k.anonymous && k.key?.enabled).map(k => ({ provider_id: m.provider_id, key_id: k.id }));
type Common = { disabled: boolean; rates: MappingRatesController; routing: ModelRoutingController; onOpen: (id: string, tab: DetailTab) => void };

function UnavailableSection({ label, count, children }: { label: string; count: number; children: ReactNode }) {
  return count > 0 && <details className="unavailable-section" aria-label={label}>
    <summary><ChevronDown size={14} /><span>{label}</span><span className="source-count">{count}</span></summary>
    <div className="unavailable-section-body">{children}</div>
  </details>;
}

function MappingKey({ source, candidate: m, rates, children, disabled, onOpen }: Pick<Common, 'rates' | 'disabled' | 'onOpen'> & { source: SourceKey; candidate: ModelCandidate; children: ReactNode }) {
  const id = rateID({ provider_id: m.provider_id, key_id: source.id }), value = rates.values[id];
  const failure = rates.errors[id] || (value?.failure && (rateFailureText[value.failure] ?? '查询失败，分组倍率未知。'));
  const keyState = keyAvailability(source);
  const state = m.provider_enabled ? keyState : m.availability;
  // Billing refreshes must never overwrite the workspace's scheduling state.
  const key = rates.keys[id] ?? source.key;
  const multiplier = effectiveRouteMultiplier({ ...m, key_ids: [source.id], keys: key ? [{ ...key, enabled: source.key?.enabled ?? false, state: source.key?.state ?? 'disabled' }] : [] });
  return <div className={`mapping-key-card mapping-key-rate availability-${state.state}`} role="group" aria-label={`${m.provider_id} · ${source.anonymous ? '匿名来源' : `Key ${source.id}`}`}>
    <div className="mapping-source-header">
      <button className={`candidate tone-${m.tone}`} disabled={disabled} title={m.label} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}><strong>{m.provider_id}</strong></button>
      <span className={`channel-status state-${state.state}`} title={`${m.provider_enabled ? '供应商已启用' : '供应商已停用'} · ${state.reason}`}>{state.label}</span>
    </div>
    <div className="mapping-key-name" aria-label={`来源 Key ${source.id}`}><span>{source.anonymous ? '来源' : 'Key'}</span><strong className="mono">{source.anonymous ? '匿名来源' : source.id}</strong></div>
    <div className="mapping-upstream"><span>上游模型</span><span className="mono">{m.upstream}</span></div>
    <div className="mapping-rate-summary"><span>排序倍率</span><strong className="mapping-effective-rate" title={`供应商 ${m.cost_multiplier}× · 模型 ${m.multiplier}×`}>{multiplier == null ? '未知' : `${Number(multiplier.toPrecision(6))}×`}</strong></div>
    {!source.anonymous && source.key && (rates.pending === id ? <p className="card-hint" role="status">查询中…</p> : <KeyMultiplierView value={key} />)}
    {failure && <p className="feedback error-feedback" role="alert">{failure}</p>}
    <footer className="mapping-key-footer"><span className="mapping-key-status">{!m.provider_enabled && <span>供应商已停用</span>}{source.key && <span>{source.key.enabled ? 'Key 已启用' : 'Key 已停用'}</span>}</span>{children}</footer>
  </div>;
}

function ModelCard({ group: g, disabled, rates, routing, onOpen, settings }: Common & { group: ModelGroup; settings: () => void }) {
  const draft = routing.draft(g), status = routing.status(g);
  const candidates = draft.order.map(ref => g.candidates.find(c => candidateID(c) === routeID(ref))).filter((c): c is ModelCandidate => !!c);
  const [confirmLatency, setConfirmLatency] = useState(false);
  const canEdit = !disabled && !routing.busy && g.complete && !routing.needsRead && status?.phase !== 'failed';
  const drag = useRouteDrag(candidates.map(candidateID), canEdit && candidates.length > 1, (from, to) => { void routing.move(g, from, to); });
  const visible = drag.order.map(id => candidates.find(c => candidateID(c) === id)!);
  const dragged = drag.drag?.active ? candidates.find(c => candidateID(c) === drag.drag!.id) : null;
  const available = candidates.filter(c => c.availability.state === 'available').length;
  const unknown = !g.complete || candidates.some(c => c.availability.state === 'unknown');
  const availability = modelAvailability(g);
  const renderCandidate = (m: ModelCandidate) => {
    const i = visible.findIndex(c => candidateID(c) === candidateID(m));
    const keys = channelKeys(m.key_ids, m.keys);
    return <li key={candidateID(m)} className={`mapping-candidate availability-${m.availability.state}${dragged && candidateID(m) === candidateID(dragged) ? ' is-drag-placeholder' : ''}`} data-route-id={candidateID(m)}>
      {keys.map((source, keyIndex) => {
        const suffix = keyIndex ? ` Key ${source.id}` : '';
        return <MappingKey key={source.id} source={source} candidate={m} rates={rates} disabled={disabled || routing.busy} onOpen={onOpen}>
          <div className="mapping-route-controls" title={`渠道优先级；调整 ${m.provider_id} 的全部 ${keys.length} 个来源 Key`}>
            <span className="route-position" aria-label={`第 ${i + 1} 位`}>渠道 {i + 1}</span>
            {candidates.length > 1 && <>
              <button type="button" className="route-grip" aria-disabled={!canEdit} aria-label={`拖动 ${m.provider_id} ${m.upstream}${suffix}`} title="拖动或用方向键调整整个渠道" onPointerDown={e => drag.start(e, candidateID(m))} onKeyDown={e => { if (canEdit && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); void routing.move(g, i, i + (e.key === 'ArrowUp' ? -1 : 1)); } }}><GripVertical size={16} /></button>
              <div className="route-move"><Button variant="ghost" size="sm" disabled={!canEdit || i === 0} aria-label={`上移 ${m.provider_id} ${m.upstream}${suffix}`} onClick={() => { void routing.move(g, i, i - 1); }}><ArrowUp size={13} /></Button><Button variant="ghost" size="sm" disabled={!canEdit || i === candidates.length - 1} aria-label={`下移 ${m.provider_id} ${m.upstream}${suffix}`} onClick={() => { void routing.move(g, i, i + 1); }}><ArrowDown size={13} /></Button></div>
            </>}
          </div>
        </MappingKey>;
      })}
    </li>;
  };
  return <article className={`model-card availability-${availability}${dragged ? ' is-sorting' : ''}`} aria-label={`模型 ${g.alias}`}>
    <header className="model-section-header"><div className="model-title"><h2 className="mono">{g.alias}</h2><span className={`model-channel-count state-${availability}`} role="status" title={`${available} / ${candidates.length} 个渠道当前可用`}>{available ? `${available}/${candidates.length} 渠道可用` : unknown ? '可用性待确认' : '无有效渠道'}</span></div><Button className="model-settings-trigger" variant="ghost" size="sm" disabled={disabled || routing.busy || !g.complete} aria-label={`模型设置 ${g.alias}`} title="上下文、能力与速度" onClick={settings}><Settings2 size={15} /></Button></header>
      <div className="model-sources-body">
        <div className="route-toolbar"><select aria-label={`调度规则 ${g.alias}`} className="select-field" disabled={!canEdit} value={draft.strategy} onChange={e => { void routing.change(g, { strategy: e.target.value }); }}>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}</select>
        {candidates.length > 1 && <div className="route-quick-sort">
          <Button variant="outline" size="sm" disabled={!canEdit} onClick={() => { void routing.sortByPrice(g); }}>按倍率</Button>
          <Button variant="outline" size="sm" disabled={!canEdit || !routing.canProbe || candidates.every(c => c.availability.state !== 'available')} onClick={() => setConfirmLatency(true)}>低延迟优先</Button>
          {routing.sorting?.alias === g.alias && <span role="status">{routing.sorting.complete}/{routing.sorting.total}{routing.sorting.failures ? ` · ${routing.sorting.failures} 失败` : ''}</span>}
          {routing.sorting?.alias === g.alias && <Button variant="ghost" size="sm" onClick={routing.stopSorting}>停止</Button>}
        </div>}
        </div>
        <ul className="candidate-list" onPointerMove={drag.move} onPointerUp={drag.end} onPointerCancel={drag.cancel} onLostPointerCapture={drag.cancel}>{visible.map(renderCandidate)}</ul>
      </div>
    {status && <div className={`route-save-state phase-${status.phase}`}>
      <span role={status.phase === 'failed' || status.phase === 'uncertain' ? 'alert' : 'status'}>{status.phase === 'saving' ? <LoaderCircle size={13} className="route-spinner" /> : status.phase === 'saved' ? <Check size={13} /> : null}{status.message}</span>
      {status.phase === 'saved' && status.undo && <Button variant="ghost" size="sm" disabled={!canEdit} onClick={() => { void routing.undo(g); }}><Undo2 size={13} />撤销</Button>}
      {status.phase === 'failed' && <><Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.retry(g); }}>重试保存</Button><Button size="sm" variant="ghost" disabled={disabled || routing.busy} onClick={() => routing.discard(g)}>放弃修改</Button></>}
      {status.phase === 'uncertain' && <Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.refresh(); }}>重新读取配置</Button>}
    </div>}
    <ConfirmAction open={confirmLatency} title="测延迟并排序" busy={routing.busy} onCancel={() => setConfirmLatency(false)} onConfirm={() => { setConfirmLatency(false); void routing.sortByLatency(g); }}>
      <p>将对 {candidates.filter(c => c.availability.state === 'available').length} 个可用渠道各发送 1 次推理请求，可能产生费用。</p>
    </ConfirmAction>
    <span className="sr-only" role="status">{drag.announcement}</span>
    {dragged && drag.drag && <div className="route-drag-preview" aria-hidden="true" style={{ left: drag.drag.x - drag.drag.offsetX, top: drag.drag.y - drag.drag.offsetY, width: drag.drag.width }}><GripVertical size={16} /><strong>{dragged.provider_id}</strong><span className="mono">{dragged.upstream}</span><span>第 {drag.drag.to + 1} 位</span></div>}
  </article>;
}

export function ModelsView({ summaries, loading, disabled, rates, routing, onOpen, onShowProviders, transport, children }: Common & {
  transport?: ProviderTransport; summaries: ProviderSummary[]; loading: boolean; onShowProviders: () => void; children?: ReactNode;
}) {
  const [settingsID, setSettingsID] = useState<string | null>(null);
  const groups = aggregateModels(summaries), pending = loading || summaries.some(s => s.workspace === undefined);
  const split = visibleModels(groups);
  const readFailed = summaries.some(s => s.workspace === null);
  const locked = disabled || rates.busy || routing.busy;
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1></div><div className="mapping-rate-actions"><Button variant="outline" size="sm" disabled={locked} onClick={() => { void routing.refresh(); }}>刷新配置</Button><Button variant="outline" size="sm" disabled={locked || !groups.length} onClick={() => { void rates.refresh(groups.flatMap(g => g.candidates.flatMap(sourceTargets))); }}><RefreshCw size={14} />{rates.busy ? '正在查询分组倍率…' : '刷新分组倍率'}</Button></div></header>
    {children}
    {settingsID && <ModelSettingsDialog key={settingsID} id={settingsID} group={groups.find(g => g.alias === settingsID)} transport={transport} active={!disabled || routing.busy} close={() => setSettingsID(null)} saved={() => routing.refresh()} />}
    {readFailed && <p className="feedback error-feedback" role="alert">部分供应商状态读取失败，渠道可用性可能不完整；请刷新配置重试。</p>}
    {routing.error && <p className="feedback error-feedback" role="alert">{routing.error}</p>}
    {routing.needsRead && <p className="card-hint">保存结果需要核对，请重新读取配置后继续修改。</p>}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : readFailed ? '无法确认模型映射' : '还没有模型映射'}</h2>{!pending && <Button disabled={locked} onClick={onShowProviders}>前往供应商</Button>}</div>
      : <><section className="model-grid" aria-label="模型别名">{split.visible.map(g => <ModelCard key={g.alias} group={g} disabled={disabled || rates.busy} rates={rates} routing={routing} onOpen={onOpen} settings={() => setSettingsID(g.alias)} />)}</section>
      <UnavailableSection label="当前不可用的模型" count={split.unavailable.length}><section className="model-grid" aria-label="不可用模型别名">{split.unavailable.map(g => <ModelCard key={g.alias} group={g} disabled={disabled || rates.busy} rates={rates} routing={routing} onOpen={onOpen} settings={() => setSettingsID(g.alias)} />)}</section></UnavailableSection></>}
  </div>;
}
