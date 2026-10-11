import type { ReactNode } from 'react';
import { ArrowLeftRight } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { aggregateModels, type DetailTab, type ProviderSummary } from '../providers/provider-summary.model';

/** Read-only cross-provider view: which providers can serve each model alias. Editing happens in the provider's 模型 tab. */
export function ModelsView({ summaries, loading, disabled, onOpen, onShowProviders, children }: {
  summaries: ProviderSummary[]; loading: boolean; disabled: boolean; onOpen: (id: string, tab: DetailTab) => void; onShowProviders: () => void; children?: ReactNode;
}) {
  const groups = aggregateModels(summaries);
  const pending = loading || summaries.some(s => s.workspace === undefined);
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">路由</div><h1>模型映射</h1><p>Agent 请求的模型别名会路由到下列供应商。点击供应商可编辑它的映射。</p></div></header>
    {children}
    {!groups.length ? <div className="empty-state"><ArrowLeftRight size={32} /><h2>{pending ? '正在读取模型映射' : '还没有模型映射'}</h2>
      {!pending && <><p>在供应商详情的「模型」分区导入目录或手动添加映射。</p><Button disabled={disabled} onClick={onShowProviders}>前往供应商</Button></>}</div>
      : <section className="model-grid" aria-label="模型别名">{groups.map(g => <article key={g.alias} className="detail-card model-card">
        <header><h2 className="mono">{g.alias}</h2><span className="cell-hint">{g.candidates.length} 个供应商</span></header>
        <ul className="candidate-list">{g.candidates.map(m => <li key={m.provider_id}>
          <button className={`candidate tone-${m.tone}`} disabled={disabled} title={m.label} aria-label={`${m.provider_id}（${m.label}）上游 ${m.upstream}，编辑映射`} onClick={() => onOpen(m.provider_id, 'models')}>
            <span className="mini-dot" aria-hidden="true" /><strong>{m.provider_id}</strong><span className="mono">{m.upstream}</span>
            {m.tools && <span className="tag">工具</span>}{m.streaming && <span className="tag">流式</span>}
          </button>
        </li>)}</ul>
      </article>)}</section>}
  </div>;
}
