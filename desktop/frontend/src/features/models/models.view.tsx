import type { ProviderTransport } from '../providers/provider.model';
import { useState, type HTMLAttributes, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { ArrowLeftRight, ArrowUp, ArrowDown, Check, GripVertical, LoaderCircle, RefreshCw, Settings2, Undo2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { aggregateModels, type DetailTab, type ModelCandidate, type ModelGroup, type ProviderSummary } from '../providers/provider-summary.model';
import { channelKeys, keyAvailability, type SourceKey } from '../providers/channel-availability.model';
import { rateFailureText } from '../providers/provider-account.model';
import { rateID, type MappingRatesController, type RateTarget } from './use-mapping-rates';
import { modelAvailability, visibleModels } from './model-availability.model';
import { effectiveRouteMultiplier, modelSources, sourceID, routingRules, routeID } from './model-routing.model';
import type { ModelRoutingController } from './use-model-routing';
import { useRouteDrag } from './use-route-drag';
import { modelFamily } from './model-appearance';
import { ModelSettingsDialog } from './model-settings.view';
import './models.css';
const candidateID = (m: ModelCandidate) => routeID({ provider_id: m.provider_id, model: m.upstream });
const sourceTargets = (m: ModelCandidate): RateTarget[] => channelKeys(m.key_ids, m.keys).filter(k => !k.anonymous && k.key?.enabled).map(k => ({ provider_id: m.provider_id, key_id: k.id }));
type Common = { disabled: boolean; rates: MappingRatesController; routing: ModelRoutingController; onOpen: (id: string, tab: DetailTab) => void };

function MappingKey({ source, candidate: m, rates, children, disabled, onOpen, dragProps, sortable }: Pick<Common, 'rates' | 'disabled' | 'onOpen'> & { source: SourceKey; candidate: ModelCandidate; children?: ReactNode; sortable?: boolean; dragProps?: HTMLAttributes<HTMLDivElement> }) {
  const id = rateID({ provider_id: m.provider_id, key_id: source.id }), value = rates.values[id];
  const failure = rates.errors[id] || (value?.failure && (rateFailureText[value.failure] ?? '查询失败，分组倍率未知。'));
  const keyState = keyAvailability(source);
  const state = m.provider_enabled ? keyState : m.availability;
  // Billing refreshes must never overwrite the workspace's scheduling state.
  const key = rates.keys[id] ?? source.key;
  const multiplier = effectiveRouteMultiplier({ ...m, key_ids: [source.id], keys: key ? [{ ...key, enabled: source.key?.enabled ?? false, state: source.key?.state ?? 'disabled' }] : [] });
  const rateDetail = `供应商 ${m.cost_multiplier}× · 模型 ${m.multiplier}× · 分组倍率 ${key?.rate?.multiplier == null ? '未知' : `${key.rate.multiplier}×`} · 相对倍率 ${key?.effective_multiplier == null ? '未知' : `${key.effective_multiplier}×`}${key?.rate?.failure ? key.rate.multiplier == null ? ' · 刷新失败，尚无有效缓存。' : ' · 刷新失败，沿用上次成功缓存。' : ''}`;
  const group = key?.rate?.group_name || key?.rate?.group;
  return <div className={`mapping-key-card mapping-key-rate availability-${state.state}`} role="group" data-source-key={source.id} data-sortable={sortable || undefined} {...dragProps} aria-label={`${m.provider_id} · ${source.anonymous ? '匿名来源' : `Key ${source.id}`}`}>
    <div className="mapping-key-identity">
      <div className="mapping-source-header">
        <span className={`channel-status state-${state.state}`} title={`${m.provider_enabled ? '供应商已启用' : '供应商已停用'} · ${state.reason}`}><span className="sr-only">{state.label}</span></span>
        <button className={`candidate tone-${m.tone}`} disabled={disabled} title={`${m.provider_id} · ${m.upstream}`} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}><strong>{m.provider_id}</strong></button>
      </div>
      <div className="mapping-key-name" aria-label={`来源 Key ${source.id}`} title={source.anonymous ? '匿名来源' : source.id}>{source.anonymous ? '匿名来源' : source.id}</div>
      <span className="mapping-key-status sr-only">{source.key && (source.key.enabled ? 'Key 已启用' : 'Key 已停用')}</span>
    </div>
    <div className="mapping-key-feedback">
      {(state.state !== 'available' || source.key?.state === 'degraded') && <span className="mapping-key-issue">{state.label}</span>}
      {failure && <span className="mapping-rate-failure" role="alert" title={failure}>倍率刷新失败<span className="sr-only">：{failure}</span></span>}
    </div>
    <div className="mapping-key-pricing">
      <strong className="mapping-effective-rate" tabIndex={0} aria-label={`排序倍率 ${multiplier == null ? '未知' : `${Number(multiplier.toPrecision(6))}倍`}`} title={rateDetail}>{rates.pending === id ? <LoaderCircle size={14} className="route-spinner" aria-label="查询中" /> : multiplier == null ? '—' : `${Number(multiplier.toPrecision(6))}×`}</strong>
      <span className="mapping-key-group" aria-label={`分组 ${group || '未知'}`} title={group || '分组未知'}>{group || '—'}</span>
    </div>
    {children}
  </div>;

}

function ModelCard({ group: g, disabled, rates, routing, onOpen, settings }: Common & { group: ModelGroup; settings: () => void }) {
  const draft = routing.draft(g), status = routing.status(g);
  const candidates = draft.order.map(ref => g.candidates.find(c => candidateID(c) === routeID(ref))).filter((c): c is ModelCandidate => !!c);
  const [confirmLatency, setConfirmLatency] = useState(false);
  const canEdit = !disabled && !routing.busy && g.complete && !routing.needsRead && status?.phase !== 'failed';
  const sources = modelSources(g, draft.source_order);
  const drag = useRouteDrag(sources.map(v => sourceID(v.ref)), canEdit && sources.length > 1, (from, to) => { void routing.move(g, from, to); });
  const visible = drag.order.map(id => sources.find(v => sourceID(v.ref) === id)!);
  const dragged = drag.drag?.active ? sources.find(v => sourceID(v.ref) === drag.drag!.id) : null;
  const available = candidates.filter(c => c.availability.state === 'available').length;
  const unknown = !g.complete || candidates.some(c => c.availability.state === 'unknown');
  const availability = modelAvailability(g);
  const renderSource = ({ candidate: m, source, ref }: typeof sources[number], i: number) => {
    const id = sourceID(ref), suffix = ` Key ${source.id}`;
    const move = (offset: number) => { void routing.move(g, i, i + offset); };
    return <li key={id} className={`mapping-candidate${dragged && sourceID(dragged.ref) === id ? ' is-drag-placeholder' : ''}`} data-route-id={id}>
      <MappingKey source={source} candidate={m} rates={rates} disabled={disabled || routing.busy} onOpen={onOpen} sortable={canEdit && sources.length > 1} dragProps={{
        tabIndex: canEdit && sources.length > 1 ? 0 : undefined,
        onPointerDown: e => drag.start(e, id),
        onKeyDown: e => { if (e.target === e.currentTarget && canEdit && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); move(e.key === 'ArrowUp' ? -1 : 1); } },
      }}>
        <div className="mapping-route-controls">
          <span className="sr-only" aria-label={`第 ${i + 1} 位`}>Key {i + 1}</span>
          {sources.length > 1 && <>
            <button type="button" className="route-grip" aria-disabled={!canEdit} aria-label={`拖动 ${m.provider_id} ${m.upstream}${suffix}`} title="拖动或用方向键调整此 Key" onKeyDown={e => { if (canEdit && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); move(e.key === 'ArrowUp' ? -1 : 1); } }}><GripVertical size={16} /></button>
            <div className="route-move"><Button variant="ghost" size="sm" disabled={!canEdit || i === 0} aria-label={`上移 ${m.provider_id} ${m.upstream}${suffix}`} onClick={() => move(-1)}><ArrowUp size={13} /></Button><Button variant="ghost" size="sm" disabled={!canEdit || i === sources.length - 1} aria-label={`下移 ${m.provider_id} ${m.upstream}${suffix}`} onClick={() => move(1)}><ArrowDown size={13} /></Button></div>
          </>}
        </div>
      </MappingKey>
    </li>;
  };
  return <article className={`model-card availability-${availability}${dragged ? ' is-sorting' : ''}`} data-model-family={modelFamily(g)} aria-label={`模型 ${g.alias}`}>
    <header className="model-section-header"><div className="model-title"><h2 className="mono">{g.alias}</h2><span className={`model-channel-count state-${availability}`} role="status" title={`${available} / ${candidates.length} 个渠道当前可用`}>{available ? <>{available}/{candidates.length}<span className="sr-only"> 渠道可用</span></> : unknown ? '可用性待确认' : '无有效渠道'}</span></div>
        <div className="route-toolbar"><select aria-label={`调度规则 ${g.alias}`} className="select-field" disabled={!canEdit} value={draft.strategy} onChange={e => { void routing.change(g, { strategy: e.target.value }); }}>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}</select>
        {sources.length > 1 && <div className="route-quick-sort">
          <Button variant="outline" size="sm" disabled={!canEdit} onClick={() => { void routing.sortByPrice(g); }}>按倍率</Button>
          <Button variant="outline" size="sm" disabled={!canEdit || !routing.canProbe || candidates.every(c => c.availability.state !== 'available')} onClick={() => setConfirmLatency(true)}>低延迟优先</Button>
          {routing.sorting?.alias === g.alias && <span role="status">{routing.sorting.complete}/{routing.sorting.total}{routing.sorting.failures ? ` · ${routing.sorting.failures} 失败` : ''}</span>}
          {routing.sorting?.alias === g.alias && <Button variant="ghost" size="sm" onClick={routing.stopSorting}>停止</Button>}
        </div>}
        </div>
      <Button className="model-settings-trigger" variant="ghost" size="sm" disabled={disabled || routing.busy || !g.complete} aria-label={`模型设置 ${g.alias}`} title="上下文、能力与速度" onClick={settings}><Settings2 size={15} /></Button></header>
      <div className="model-sources-body">
        <ul className="candidate-list" onPointerMove={drag.move} onPointerUp={drag.end} onPointerCancel={drag.cancel} onLostPointerCapture={e => { if (e.target === e.currentTarget) drag.cancel(); }} onClickCapture={drag.click}>{visible.map(renderSource)}</ul>
      </div>
    {status && <div className={`route-save-state phase-${status.phase}`}>
      <span role={status.phase === 'failed' || status.phase === 'uncertain' ? 'alert' : 'status'}>{status.phase === 'saving' ? <LoaderCircle size={13} className="route-spinner" /> : status.phase === 'saved' ? <Check size={13} /> : null}{status.message}</span>
      {status.phase === 'saved' && status.undo && <Button variant="ghost" size="sm" disabled={!canEdit} onClick={() => { void routing.undo(g); }}><Undo2 size={13} />撤销</Button>}
      {status.phase === 'failed' && <><Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.retry(g); }}>重试保存</Button><Button size="sm" variant="ghost" disabled={disabled || routing.busy} onClick={() => routing.discard(g)}>放弃修改</Button></>}
      {status.phase === 'uncertain' && <Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.refresh(); }}>重新读取配置</Button>}
    </div>}
    <ConfirmAction open={confirmLatency} title="测延迟并排序" busy={routing.busy} onCancel={() => setConfirmLatency(false)} onConfirm={() => { setConfirmLatency(false); void routing.sortByLatency(g); }}>
      <p>将对 {sources.filter(v => v.candidate.provider_enabled && (v.source.anonymous || v.source.key?.enabled && ['ready', 'degraded'].includes(v.source.key.state))).length} 个可用 Key 各发送 1 次推理请求，可能产生费用。</p>
    </ConfirmAction>
    <span className="sr-only" role="status">{drag.announcement}</span>
    {dragged && drag.drag && createPortal(<div className="route-drag-preview" aria-hidden="true" style={{ left: drag.drag.x - drag.drag.offsetX, top: drag.drag.y - drag.drag.offsetY, width: drag.drag.width }}>
      <MappingKey source={dragged.source} candidate={dragged.candidate} rates={rates} disabled onOpen={onOpen}><div className="mapping-route-controls"><GripVertical size={16} /></div></MappingKey>
    </div>, document.body)}
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
  return <div className="workspace models-workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1></div><div className="mapping-rate-actions"><Button variant="outline" size="sm" disabled={locked} onClick={() => { void routing.refresh(); }}>刷新配置</Button><Button variant="outline" size="sm" disabled={locked || !groups.length} onClick={() => { void rates.refresh(groups.flatMap(g => g.candidates.flatMap(sourceTargets))); }}><RefreshCw size={14} />{rates.busy ? '正在查询分组倍率…' : '刷新分组倍率'}</Button></div></header>
    {children}
    {settingsID && <ModelSettingsDialog key={settingsID} id={settingsID} group={groups.find(g => g.alias === settingsID)} transport={transport} active={!disabled || routing.busy} close={() => setSettingsID(null)} saved={() => routing.refresh()} />}
    {readFailed && <p className="feedback error-feedback" role="alert">部分供应商状态读取失败，渠道可用性可能不完整；请刷新配置重试。</p>}
    {routing.error && <p className="feedback error-feedback" role="alert">{routing.error}</p>}
    {routing.needsRead && <p className="card-hint">保存结果需要核对，请重新读取配置后继续修改。</p>}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : readFailed ? '无法确认模型映射' : '还没有模型映射'}</h2>{!pending && <Button disabled={locked} onClick={onShowProviders}>前往供应商</Button>}</div>
      : <section className="model-grid" aria-label="模型别名" tabIndex={0}>{[...split.visible, ...split.unavailable].map(g => <ModelCard key={g.alias} group={g} disabled={disabled || rates.busy} rates={rates} routing={routing} onOpen={onOpen} settings={() => setSettingsID(g.alias)} />)}</section>}
  </div>;
}
