import type { CatalogRates, ReferenceCost } from '../../desktop/contract.generated';
import { referenceCostReason } from './log-reference-cost.model';

const rates: [keyof CatalogRates, string][] = [['input', '输入'], ['output', '输出'], ['cache_read', '缓存读取'], ['cache_write', '缓存写入'], ['reasoning', '思考']];
export function ReferenceCostDetail({ cost }: { cost: ReferenceCost }) {
  return <section aria-label="参考价格快照"><h3 className="log-section-title">参考价格快照</h3><dl className="log-properties">
    <div><dt>来源</dt><dd>{cost.source} · {cost.provider_id}/{cost.model_id}</dd></div>
    <div><dt>获取时间</dt><dd>{cost.fetched_at ? new Date(cost.fetched_at * 1000).toLocaleString() : '未知'}{cost.stale ? ' · 已过期' : ''}</dd></div>
    <div><dt>目录版本</dt><dd className="mono">{cost.catalog_version || '未知'}</dd></div>
    {cost.usd == null && <div><dt>未计算原因</dt><dd>{referenceCostReason(cost.reason)}</dd></div>}
    {cost.pricing && [{ label: '基础', rates: cost.pricing.rates }, ...cost.pricing.tiers.map(tier => ({ label: `输入 > ${tier.above_tokens.toLocaleString()} Token`, rates: tier.rates }))].map(band => <div key={band.label}><dt>{band.label} · USD / 百万 Token</dt><dd>{rates.map(([key, label]) => <span className="block" key={key}>{label}：{band.rates[key] ?? '—'}</span>)}</dd></div>)}
  </dl></section>;
}
