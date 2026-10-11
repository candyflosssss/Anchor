import type { ReactNode } from 'react';
import { ArrowRight, CircleCheck, Plus } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { HealthPill } from '../providers/provider-detail.view';
import { attention, overviewTotals, type DetailTab, type ProviderDialog, type ProviderSummary } from '../providers/provider-summary.model';

type Open = (id: string, tab?: DetailTab, dialog?: ProviderDialog) => void;

/** Landing page: totals, a "needs attention" queue with one-click fixes, and a compact provider list. */
export function Overview({ summaries, loading, disabled, onOpen, onCreate, onShowProviders, onShowModels, children }: {
  summaries: ProviderSummary[]; loading: boolean; disabled: boolean; onOpen: Open; onCreate: () => void; onShowProviders: () => void; onShowModels: () => void; children?: ReactNode;
}) {
  const t = overviewTotals(summaries);
  const items = attention(summaries);
  const pending = loading || t.pending;
  const metrics: [string, string, string, (() => void)?][] = [
    ['供应商', `${t.enabled} / ${t.providers}`, '已启用 / 全部', onShowProviders],
    ['模型别名', pending ? '…' : String(t.aliases), pending ? '读取中' : `${t.routes} 条供应商路由`, onShowModels],
    ['请求', pending ? '…' : t.requests.toLocaleString(), pending ? '读取中' : t.failures ? `${t.failures} 次失败` : '无失败记录'],
    ['估算费用', pending ? '…' : `$${t.usd.toFixed(t.usd < 1 ? 4 : 2)}${t.usdPartial ? '+' : ''}`, '按当前单价估算，非账单'],
  ];
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">工作台</div><h1>概览</h1><p>当前实例的供应商健康、模型和用量一览。</p></div><Button disabled={disabled} onClick={onCreate}><Plus />添加供应商</Button></header>
    {children}
    <section className="metric-grid" aria-label="关键指标">
      {metrics.map(([label, value, hint, go]) => {
        const body = <><span className="metric-label">{label}</span><strong className="metric-value">{value}</strong><span className="metric-hint">{hint}</span></>;
        return go ? <button key={label} className="metric-card" disabled={disabled} onClick={go} aria-label={`${label}：${value}，${hint}`}>{body}</button> : <div key={label} className="metric-card">{body}</div>;
      })}
    </section>
    <div className="overview-columns">
      <section className="detail-card" aria-labelledby="attention-title">
        <header className="card-toolbar"><h2 id="attention-title">需要处理</h2>{!pending && <span className="cell-hint">{items.length ? `${items.length} 项` : ''}</span>}</header>
        {pending && !items.length ? <p className="card-hint" role="status">正在检查每个供应商…</p>
          : !items.length ? <div className="empty-state compact"><CircleCheck size={22} /><p>{t.providers ? '所有已启用的供应商都已配置并测试通过。' : '还没有供应商。'}</p></div>
          : <ul className="attention-list">{items.map(item => <li key={`${item.id}-${item.code}`} className="attention-item">
            <span className={`mini-dot tone-${item.tone}`} aria-hidden="true" />
            <div className="attention-text"><strong>{item.id} · {item.title}</strong><span>{item.detail}</span></div>
            <Button size="sm" variant={item.tone === 'bad' ? 'default' : 'outline'} disabled={disabled} aria-label={`${item.action}：${item.id}`} onClick={() => onOpen(item.id, item.tab, item.dialog)}>{item.action}</Button>
          </li>)}</ul>}
      </section>
      <section className="detail-card" aria-labelledby="overview-providers">
        <header className="card-toolbar"><h2 id="overview-providers">供应商</h2><Button variant="ghost" size="sm" disabled={disabled} onClick={onShowProviders}>全部<ArrowRight /></Button></header>
        {!summaries.length ? <p className="card-hint">{loading ? '读取中…' : '添加一个上游服务后，这里会显示它的健康状态。'}</p>
          : <ul className="compact-list">{summaries.map(s => <li key={s.provider.id}>
            <button disabled={disabled} onClick={() => onOpen(s.provider.id)}>
              <span className="provider-avatar" aria-hidden="true">{Array.from(s.provider.id)[0]}</span>
              <span className="compact-name">{s.provider.id}<span className="cell-hint">{s.workspace ? `${s.keyCount ? `${s.enabledKeys}/${s.keyCount} Key` : '匿名'} · ${s.modelCount} 个模型` : ''}</span></span>
              <HealthPill tone={s.tone} label={s.label} />
            </button>
          </li>)}</ul>}
      </section>
    </div>
  </div>;
}
