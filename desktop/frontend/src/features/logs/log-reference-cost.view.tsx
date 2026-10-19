import type { CatalogRates, CostAdjustment, ReferenceCost } from '../../desktop/contract.generated';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../../components/ui/tooltip';
import { adjustmentValue, costMultiplier, referenceCostReason, referenceUSD } from './log-reference-cost.model';

const rates: [keyof CatalogRates, string][] = [['input', '非缓存输入'], ['output', '普通输出'], ['cache_read', '缓存读取'], ['cache_write', '缓存写入'], ['reasoning', '思考输出']];
const partLabels = Object.fromEntries(rates);
export function CostBreakdown({ adjustment }: { adjustment: CostAdjustment }) {
  const a = adjustment;
  return <div className="log-cost-breakdown">
    <p>逐次：原始费用 × 供应商倍率 × 分组倍率</p>
    <p>供应商 {costMultiplier(a.supplier_multiplier, a.supplier_mixed)} · 分组 {costMultiplier(a.group_multiplier, a.group_mixed)}</p>
    <p>最终参考费用：{adjustmentValue(a)}{a.reason && ` · ${referenceCostReason(a.reason)}`}</p>
    {(a.supplier_mixed || a.group_mixed) && <p>多次尝试分别计算后求和；各次倍率见请求详情。</p>}
    {!a.group_mixed && <p>分组：{a.group_name || a.group || '未观测'} · 口径：{a.basis || '未知'}<br />观测时间：{a.observed_at || '未知'}{a.failure && ` · 最近刷新失败：${a.failure}（保留上次观测）`}</p>}
    <div className="log-cost-parts">{(a.parts ?? []).map(part => <div className="log-cost-part" key={part.kind}>
      <strong>{part.includes_reasoning ? '输出（含未拆分思考）' : partLabels[part.kind] ?? part.kind}</strong><span>{part.tokens == null ? '未知' : part.tokens.toLocaleString()} Token</span>
      <span>单价：{part.mixed_rate ? '逐次阶梯' : part.rate ?? '未知'} USD / 百万 Token</span>
      <span>单价倍率：{costMultiplier(part.rate_multiplier, part.mixed_rate)}</span>
      <span>原始：{part.usd == null ? '未知' : referenceUSD(part.usd)}</span><span>最终：{part.final_usd == null ? '未知' : referenceUSD(part.final_usd)}</span>
    </div>)}</div>
    <p className="log-cost-footnote">单价倍率相对同次非缓存输入，最多保留 12 位小数。思考未单独报告且同价时随输出计价；输入已含缓存、输出已含思考，分区不重复计费；供应商与分组倍率作用于每个乘区。仅供参考，不是实际账单。</p>
  </div>;
}
export function CostCell({ kind, cost }: { kind: 'originalCost' | 'supplierMultiplier' | 'groupMultiplier' | 'referenceCost'; cost?: ReferenceCost | null }) {
  if (!cost) return <span className="mono">—</span>;
  const a = cost.adjustment;
  const value = kind === 'originalCost' ? cost.usd == null ? '未知' : referenceUSD(cost.usd)
    : kind === 'referenceCost' ? adjustmentValue(a)
      : !a ? '未记录' : kind === 'supplierMultiplier' ? costMultiplier(a.supplier_multiplier, a.supplier_mixed) : costMultiplier(a.group_multiplier, a.group_mixed);
  return <TooltipProvider delayDuration={200}><Tooltip><TooltipTrigger asChild>
    <button type="button" className="log-cost-trigger mono" aria-label={`费用乘区明细：${value}`} onClick={event => event.stopPropagation()}>{value}</button>
  </TooltipTrigger><TooltipContent className="log-cost-tooltip" side="bottom" collisionPadding={16}>
    <p className="font-medium">参考费用乘区 · USD</p><p>原始参考费用：{cost.usd == null ? `未知 · ${referenceCostReason(cost.reason)}` : referenceUSD(cost.usd)}</p>
    {a ? <CostBreakdown adjustment={a} /> : <p>历史记录未保存倍率与乘区，不按当前配置补算。</p>}
  </TooltipContent></Tooltip></TooltipProvider>;
}
export function ReferenceCostDetail({ cost }: { cost: ReferenceCost }) {
  return <section aria-label="参考价格快照"><h3 className="log-section-title">参考价格快照</h3><dl className="log-properties">
    <div><dt>来源</dt><dd>{cost.source} · {cost.provider_id}/{cost.model_id}</dd></div>
    <div><dt>获取时间</dt><dd>{cost.fetched_at ? new Date(cost.fetched_at * 1000).toLocaleString() : '未知'}{cost.stale ? ' · 已过期' : ''}</dd></div>
    <div><dt>目录版本</dt><dd className="mono">{cost.catalog_version || '未知'}</dd></div>
    {cost.usd == null && <div><dt>未计算原因</dt><dd>{referenceCostReason(cost.reason)}</dd></div>}
    {cost.pricing && [{ label: '基础', rates: cost.pricing.rates }, ...cost.pricing.tiers.map(tier => ({ label: `输入 > ${tier.above_tokens.toLocaleString()} Token`, rates: tier.rates }))].map(band => <div key={band.label}><dt>{band.label} · USD / 百万 Token</dt><dd>{rates.map(([key, label]) => <span className="block" key={key}>{label}：{band.rates[key] ?? '—'}</span>)}</dd></div>)}
  </dl>{cost.adjustment && <CostBreakdown adjustment={cost.adjustment} />}</section>;
}
