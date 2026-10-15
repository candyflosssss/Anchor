import { useState, type ReactNode } from 'react';
import { ArrowLeftRight, ArrowUp, ArrowDown, Check, ChevronDown, GripVertical, LoaderCircle, RefreshCw, Settings2, Undo2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { aggregateModels, type DetailTab, type ModelCandidate, type ModelGroup, type ProviderSummary } from '../providers/provider-summary.model';
import { KeyMultiplierView } from '../providers/key-multiplier.view';
import { rateFailureText } from '../providers/provider-account.model';
import { rateID, type MappingRatesController, type RateTarget } from './use-mapping-rates';
import { routingRules, routeID, ruleLabel } from './model-routing.model';
import type { ModelRoutingController } from './use-model-routing';
import { useRouteDrag } from './use-route-drag';
import { ModelSettingsDialog } from './model-settings.view';
import './models.css';
const candidateID = (m: ModelCandidate) => routeID({ provider_id: m.provider_id, model: m.upstream });
const sourceTargets = (m: ModelCandidate): RateTarget[] => (m.key_ids.length ? m.key_ids : m.keys.filter(k => k.enabled).map(k => k.id)).map(key_id => ({ provider_id: m.provider_id, key_id }));
type Common = { disabled: boolean; rates: MappingRatesController; routing: ModelRoutingController; onOpen: (id: string, tab: DetailTab) => void };

function ModelCard({ group: g, disabled, rates, routing, onOpen, settings }: Common & { group: ModelGroup; settings: () => void }) {
  const draft = routing.draft(g), status = routing.status(g);
  const candidates = draft.order.map(ref => g.candidates.find(c => candidateID(c) === routeID(ref))).filter((c): c is ModelCandidate => !!c);
  const [expanded, setExpanded] = useState(false);
  const canEdit = !disabled && !routing.busy && g.complete && !routing.needsRead && status?.phase !== 'failed';
  const drag = useRouteDrag(candidates.map(candidateID), canEdit && expanded && candidates.length > 1, (from, to) => { void routing.move(g, from, to); });
  const visible = drag.order.map(id => candidates.find(c => candidateID(c) === id)!);
  const dragged = drag.drag?.active ? candidates.find(c => candidateID(c) === drag.drag!.id) : null;
  const keyCount = candidates.reduce((sum, c) => sum + sourceTargets(c).length, 0);
  return <article className={`detail-card model-card${dragged ? ' is-sorting' : ''}`} aria-label={`模型 ${g.alias}`}>
    <header><h2 className="mono">{g.alias}</h2><Button className="model-settings-trigger" variant="ghost" size="sm" disabled={disabled || routing.busy || !g.complete} aria-label={`模型设置 ${g.alias}`} title="上下文、能力与思考选项" onClick={settings}><Settings2 size={15} /><span>设置</span></Button></header>
    <div className="model-card-summary"><span className="model-strategy">{ruleLabel(draft.strategy)}</span><span>{candidates.length} 个来源 · {keyCount} 个 Key</span></div>
    {!g.complete && <p className="card-hint">配置正在更新，请稍候或刷新配置。</p>}
    <details className="model-sources" open={expanded} onToggle={e => setExpanded(e.currentTarget.open)}>
      <summary><ChevronDown size={14} /><span>供应商与 Key</span><span className="source-count">{candidates.length}</span></summary>
      <div className="model-sources-body">
        <div className="route-settings-body"><label htmlFor={`strategy-${g.alias}`}>调度规则</label><select id={`strategy-${g.alias}`} className="select-field" disabled={!canEdit} value={draft.strategy} onChange={e => { void routing.change(g, { strategy: e.target.value }); }}>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}</select><p className="card-hint">{routingRules[draft.strategy as keyof typeof routingRules]?.hint}</p></div>
        {candidates.length > 1 && <p id={`sort-hint-${g.alias}`} className="route-sort-hint">拖动调整顺序，松手自动保存。也可用上下按钮，或聚焦拖柄后按 ↑ / ↓。</p>}
        <ul className="candidate-list" onPointerMove={drag.move} onPointerUp={drag.end} onPointerCancel={drag.cancel} onLostPointerCapture={drag.cancel}>{visible.map((m, i) => <li key={candidateID(m)} className={`mapping-candidate${dragged && candidateID(m) === candidateID(dragged) ? ' is-drag-placeholder' : ''}`} data-route-id={candidateID(m)}>
          <div className="mapping-source-header">
            <span className="route-position" aria-label={`第 ${i + 1} 位`}>{i + 1}</span>
            {candidates.length > 1 && <button type="button" className="route-grip" aria-disabled={!canEdit} aria-label={`拖动 ${m.provider_id} ${m.upstream}`} aria-describedby={`sort-hint-${g.alias}`} title="拖动或用方向键调整顺序" onPointerDown={e => drag.start(e, candidateID(m))} onKeyDown={e => { if (canEdit && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); void routing.move(g, i, i + (e.key === 'ArrowUp' ? -1 : 1)); } }}><GripVertical size={16} /></button>}
            <button className={`candidate tone-${m.tone}`} disabled={disabled || routing.busy} title={m.label} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}><strong>{m.provider_id}</strong><span className="mono">{m.upstream}</span></button>
            {candidates.length > 1 && <div className="route-move"><Button variant="ghost" size="sm" disabled={!canEdit || i === 0} aria-label={`上移 ${m.provider_id} ${m.upstream}`} onClick={() => { void routing.move(g, i, i - 1); }}><ArrowUp size={13} /></Button><Button variant="ghost" size="sm" disabled={!canEdit || i === candidates.length - 1} aria-label={`下移 ${m.provider_id} ${m.upstream}`} onClick={() => { void routing.move(g, i, i + 1); }}><ArrowDown size={13} /></Button></div>}
          </div>
          <div className="mapping-source-meta"><span>供应商倍率 <strong>{m.cost_multiplier}×</strong></span><details className="mapping-keys"><summary><ChevronDown size={12} />{sourceTargets(m).length ? `${sourceTargets(m).length} 个 Key` : '匿名来源'}</summary><div className="mapping-rates">
            {!sourceTargets(m).length && <p className="card-hint">此来源不使用 Key。</p>}
            {sourceTargets(m).map(t => {
              const id = rateID(t), value = rates.values[id], failure = rates.errors[id] || (value?.failure && (rateFailureText[value.failure] ?? '查询失败，分组倍率未知。'));
              const key = rates.keys[id] ?? m.keys.find(k => k.id === t.key_id);
              return <div className="mapping-key-rate" key={id}><span className="mapping-key-name" aria-label={`来源 Key ${t.key_id}`}><span>Key</span><strong className="mono">{t.key_id}</strong></span>{rates.pending === id ? <span>查询中…</span> : <KeyMultiplierView value={key} />}{failure && <p className="feedback error-feedback" role="alert">{failure}</p>}</div>;
            })}
          </div></details></div>
        </li>)}</ul>
      </div>
    </details>
    {status && <div className={`route-save-state phase-${status.phase}`}>
      <span role={status.phase === 'failed' || status.phase === 'uncertain' ? 'alert' : 'status'}>{status.phase === 'saving' ? <LoaderCircle size={13} className="route-spinner" /> : status.phase === 'saved' ? <Check size={13} /> : null}{status.message}</span>
      {status.phase === 'saved' && status.undo && <Button variant="ghost" size="sm" disabled={!canEdit} onClick={() => { void routing.undo(g); }}><Undo2 size={13} />撤销</Button>}
      {status.phase === 'failed' && <><Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.retry(g); }}>重试保存</Button><Button size="sm" variant="ghost" disabled={disabled || routing.busy} onClick={() => routing.discard(g)}>放弃修改</Button></>}
      {status.phase === 'uncertain' && <Button size="sm" variant="outline" disabled={disabled || routing.busy} onClick={() => { void routing.refresh(); }}>重新读取配置</Button>}
    </div>}
    <span className="sr-only" role="status">{drag.announcement}</span>
    {dragged && drag.drag && <div className="route-drag-preview" aria-hidden="true" style={{ left: drag.drag.x - drag.drag.offsetX, top: drag.drag.y - drag.drag.offsetY, width: drag.drag.width }}><GripVertical size={16} /><strong>{dragged.provider_id}</strong><span className="mono">{dragged.upstream}</span><span>第 {drag.drag.to + 1} 位</span></div>}
  </article>;
}

export function ModelsView({ summaries, loading, disabled, rates, routing, onOpen, onShowProviders, children }: Common & {
  summaries: ProviderSummary[]; loading: boolean; onShowProviders: () => void; children?: ReactNode;
}) {
  const [settingsID, setSettingsID] = useState<string | null>(null);
  const groups = aggregateModels(summaries), pending = loading || summaries.some(s => s.workspace === undefined);
  const locked = disabled || rates.busy || routing.busy;
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1></div><div className="mapping-rate-actions"><Button variant="outline" size="sm" disabled={locked} onClick={() => { void routing.refresh(); }}>刷新配置</Button><Button variant="outline" size="sm" disabled={locked || !groups.length} onClick={() => { void rates.refresh(groups.flatMap(g => g.candidates.flatMap(sourceTargets))); }}><RefreshCw size={14} />{rates.busy ? '正在查询分组倍率…' : '刷新分组倍率'}</Button></div></header>
    {children}
    {settingsID && <ModelSettingsDialog key={settingsID} id={settingsID} active={!disabled || routing.busy} close={() => setSettingsID(null)} saved={() => routing.refresh()} />}
    {routing.error && <p className="feedback error-feedback" role="alert">{routing.error}</p>}
    {routing.needsRead && <p className="card-hint">保存结果需要核对，请重新读取配置后继续修改。</p>}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : '还没有模型映射'}</h2>{!pending && <Button disabled={locked} onClick={onShowProviders}>前往供应商</Button>}</div>
      : <section className="model-grid" aria-label="模型别名">{groups.map(g => <ModelCard key={g.alias} group={g} disabled={disabled || rates.busy} rates={rates} routing={routing} onOpen={onOpen} settings={() => setSettingsID(g.alias)} />)}</section>}
  </div>;
}
