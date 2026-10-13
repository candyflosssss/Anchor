import type { ReactNode } from 'react';
import { ArrowLeftRight, RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { aggregateModels, type DetailTab, type ProviderSummary } from '../providers/provider-summary.model';

import { GroupRateObservation } from '../providers/provider-rate.view';
import type { MappingRatesController } from './use-mapping-rates';
import './models.css';

/** Read-only cross-provider view: which providers can serve each model alias. Editing happens in the provider's 模型 tab. */
export function ModelsView({ summaries, loading, disabled, rates, onOpen, onShowProviders, children }: {
  summaries: ProviderSummary[]; loading: boolean; disabled: boolean; rates: MappingRatesController; onOpen: (id: string, tab: DetailTab) => void; onShowProviders: () => void; children?: ReactNode;
}) {
  const groups = aggregateModels(summaries);
  const pending = loading || summaries.some(s => s.workspace === undefined);
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1></div><Button variant="outline" size="sm" disabled={disabled || rates.busy || !groups.length} onClick={() => void rates.refresh(groups.flatMap(g => g.candidates.map(m => m.provider_id)))}><RefreshCw size={14} />{rates.busy ? "正在查询分组倍率…" : "刷新分组倍率"}</Button></header>
    <p className="page-hint">路由倍率来自已保存的映射；平台分组倍率按默认启用 Key 查询。多个 Key 的分组可能不同，可在账户页选择 Key 核对。</p>
    {children}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : '还没有模型映射'}</h2>
      {!pending && <><Button disabled={disabled} onClick={onShowProviders}>前往供应商</Button></>}</div>
      : <section className="model-grid" aria-label="模型别名">{groups.map(g => <article key={g.alias} className="detail-card model-card">
        <header><h2 className="mono">{g.alias}</h2><span className="cell-hint">{g.candidates.length} 个供应商</span></header>
        <ul className="candidate-list">{g.candidates.map(m => <li key={m.provider_id} className="mapping-candidate">
          <button className={`candidate tone-${m.tone}`} disabled={disabled || rates.busy} title={m.label} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}>
            <span className="mini-dot" aria-hidden="true" /><strong>{m.provider_id}</strong><span className="mono">{m.upstream}</span>
            {m.tools && <span className="tag">工具</span>}{m.streaming && <span className="tag">流式</span>}
          </button>
          <div className="mapping-rates">
            <p className="mapping-route-rate">路由倍率 <strong>{m.multiplier}×</strong> · {m.strategy === 'priority' ? '优先级策略' : '轮询策略'}</p>
            {rates.pending === m.provider_id ? <p className="card-hint" role="status">正在查询默认 Key 的分组倍率…</p> : <GroupRateObservation value={rates.values[m.provider_id] ?? null} />}
            {rates.errors[m.provider_id] && <p className="feedback error-feedback" role="alert">{rates.errors[m.provider_id]}</p>}
            <div className="mapping-rate-actions"><Button variant="ghost" size="sm" disabled={disabled || rates.busy} onClick={() => void rates.refresh([m.provider_id])}>查询分组倍率</Button>
              <Button variant="ghost" size="sm" disabled={disabled || rates.busy} onClick={() => onOpen(m.provider_id, 'account')}>账户与授权</Button></div>
          </div>
        </li>)}</ul>
      </article>)}</section>}
  </div>;
}
