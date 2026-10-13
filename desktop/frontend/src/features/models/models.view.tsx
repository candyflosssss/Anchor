import type { ReactNode } from 'react';
import { ArrowLeftRight, ArrowUp, ArrowDown, GripVertical, RefreshCw, Settings2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { aggregateModels, type DetailTab, type ModelCandidate, type ProviderSummary } from '../providers/provider-summary.model';
import { rateFailureText } from '../providers/provider-account.model';
import { rateID, type MappingRatesController, type RateTarget } from './use-mapping-rates';
import { routingRules, routeID } from './model-routing.model';
import type { ModelRoutingController } from './use-model-routing';
import './models.css';
const candidateID = (m: ModelCandidate) => routeID({ provider_id: m.provider_id, model: m.upstream });
const sourceTargets = (m: ModelCandidate): RateTarget[] => (m.key_ids.length ? m.key_ids : m.keys.filter(k => k.enabled).map(k => k.id)).map(key_id => ({ provider_id: m.provider_id, key_id }));
export function ModelsView({ summaries, loading, disabled, rates, routing, onOpen, onShowProviders, children }: {
  summaries: ProviderSummary[]; loading: boolean; disabled: boolean; rates: MappingRatesController; routing: ModelRoutingController; onOpen: (id: string, tab: DetailTab) => void; onShowProviders: () => void; children?: ReactNode;
}) {
  const groups = aggregateModels(summaries);
  const pending = loading || summaries.some(s => s.workspace === undefined);
  const locked = disabled || rates.busy || routing.busy;
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1></div><div className="mapping-rate-actions"><Button variant="outline" size="sm" disabled={locked} onClick={() => void routing.refresh()}>刷新配置</Button><Button variant="outline" size="sm" disabled={locked || !groups.length} onClick={() => void rates.refresh(groups.flatMap(g => g.candidates.flatMap(sourceTargets)))}><RefreshCw size={14} />{rates.busy ? '正在查询分组倍率…' : '刷新分组倍率'}</Button></div></header>
    {children}
    {routing.error && <p className="feedback error-feedback" role="alert">{routing.error}</p>}
    {routing.notice && <p className="feedback" role="status">{routing.notice}</p>}
    {routing.needsRead && <p className="card-hint">请刷新配置后核对顺序与规则。写入不会自动重放。</p>}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : '还没有模型映射'}</h2>{!pending && <Button disabled={locked} onClick={onShowProviders}>前往供应商</Button>}</div>
      : <section className="model-grid" aria-label="模型别名">{groups.map(g => {
        const draft = routing.draft(g);
        const candidates = draft.order.map(ref => g.candidates.find(c => candidateID(c) === routeID(ref))).filter((c): c is ModelCandidate => !!c);
        const canEdit = !locked && g.complete && !routing.needsRead;
        return <article key={g.alias} className="detail-card model-card" aria-label={`模型 ${g.alias}`}>
          <header><h2 className="mono">{g.alias}</h2></header>
          {!g.complete && <p className="card-hint">候选尚未完整读取或版本不一致，请刷新配置。</p>}
          <details className="route-settings"><summary><Settings2 size={14} />调度设置</summary><div className="route-settings-body">
            <label htmlFor={`strategy-${g.alias}`}>调度规则</label><select id={`strategy-${g.alias}`} className="select-field" disabled={!canEdit} value={draft.strategy} onChange={e => routing.change(g, { strategy: e.target.value })}>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}</select>
            <p className="card-hint">{routingRules[draft.strategy as keyof typeof routingRules]?.hint}</p>
          </div></details>
          <ul className="candidate-list">{candidates.map((m, i) => <li key={candidateID(m)} className={`mapping-candidate${canEdit && candidates.length > 1 ? ' can-reorder' : ''}`} data-route-id={candidateID(m)} onDragOver={e => { if (canEdit) e.preventDefault(); }} onDrop={e => {
            e.preventDefault(); if (!canEdit) return;
            try { const [alias, id] = JSON.parse(e.dataTransfer.getData('text/plain')) as string[]; if (alias === g.alias) { const from = candidates.findIndex(c => candidateID(c) === id); if (from >= 0) routing.move(g, from, i); } } catch { /* Ignore unrelated drag data. */ }
          }}>
            <div className="mapping-source-header"><button type="button" className="route-grip" draggable={canEdit && candidates.length > 1} disabled={!canEdit || candidates.length < 2} aria-label={`拖动 ${m.provider_id} ${m.upstream}`} title={candidates.length > 1 ? '拖动调整渠道顺序' : '仅一个渠道，无需排序'} onDragStart={e => { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', JSON.stringify([g.alias, candidateID(m)])); }}><GripVertical size={16} /></button>
              <button className={`candidate tone-${m.tone}`} disabled={locked} title={m.label} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}><strong>{m.provider_id}</strong><span className="mono">{m.upstream}</span></button>
              {candidates.length > 1 && <div className="route-move"><Button variant="ghost" size="sm" disabled={!canEdit || i === 0} aria-label={`上移 ${m.provider_id} ${m.upstream}`} onClick={() => routing.move(g, i, i - 1)}><ArrowUp size={13} /></Button><Button variant="ghost" size="sm" disabled={!canEdit || i === candidates.length - 1} aria-label={`下移 ${m.provider_id} ${m.upstream}`} onClick={() => routing.move(g, i, i + 1)}><ArrowDown size={13} /></Button></div>}
            </div>
            <div className="mapping-rates">
              <p className="mapping-route-rate">路由倍率 <strong>{m.multiplier}×</strong></p>
              {!sourceTargets(m).length && <span className="mapping-key-name mono">anonymous</span>}
              {sourceTargets(m).map(t => {
                const id = rateID(t), value = rates.values[id], failure = rates.errors[id] || (value?.failure && (rateFailureText[value.failure] ?? '查询失败，分组倍率未知。'));
                return <div className="mapping-key-rate" key={id}>
                  <span className="mapping-key-name" aria-label={`来源 Key ${t.key_id}`}><span>Key</span><strong className="mono">{t.key_id}</strong></span>
                  <span className="mapping-platform-rate" title={failure || undefined}>分组倍率 <strong>{rates.pending === id ? '查询中…' : value?.rate?.multiplier != null ? `${value.rate.multiplier}×` : '—'}</strong></span>
                  {failure && <p className="feedback error-feedback" role="alert">{failure}</p>}
                </div>;
              })}
            </div>
          </li>)}</ul>
          {routing.dirty(g) && <div className="route-save"><Button disabled={!canEdit} onClick={() => void routing.save(g)}>保存顺序与规则</Button><Button variant="ghost" disabled={!canEdit} onClick={() => routing.reset(g)}>撤销修改</Button></div>}
        </article>;
      })}</section>}
  </div>;
}
