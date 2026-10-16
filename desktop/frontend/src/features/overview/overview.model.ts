import { overviewTotals, type ProviderSummary } from '../providers/provider-summary.model';

export interface OverviewMetric { label: string; value: string; hint: string; destination?: 'providers' | 'models' }

/** A failed or absent observation is not a measured zero. Partial totals are lower bounds. */
export function overviewMetrics(summaries: ProviderSummary[], loading: boolean, readFailed = false): OverviewMetric[] {
  const t = overviewTotals(summaries);
  const pending = loading || t.pending;
  const loaded = summaries.filter(s => s.workspace);
  const partial = loaded.length < summaries.length;
  const unavailable = readFailed || (summaries.length > 0 && loaded.length === 0);
  const knownEstimate = loaded.some(s => s.workspace!.estimates.some(e => e.usd !== null));
  const noUsage = loaded.every(s => !s.requests && !s.workspace!.estimates.length);
  const costUnknown = unavailable || (!knownEstimate && !noUsage);
  const value = (n: number) => pending ? '…' : unavailable ? '—' : `${n.toLocaleString()}${partial ? '+' : ''}`;
  const hint = (complete: string) => pending ? '读取中' : unavailable ? '读取失败' : partial ? '部分数据 · 统计不完整' : complete;
  return [
    { label: '供应商', value: loading ? '…' : readFailed ? '—' : `${t.enabled} / ${t.providers}`, hint: loading ? '读取中' : readFailed ? '读取失败' : '已启用 / 全部', destination: 'providers' },
    { label: '模型别名', value: value(t.aliases), hint: hint(`${t.routes} 条供应商路由`), destination: 'models' },
    { label: '请求', value: value(t.requests), hint: hint(t.failures ? `${t.failures} 次失败` : '无失败记录') },
    { label: '估算费用', value: pending ? '…' : costUnknown ? '—' : `$${t.usd.toFixed(t.usd < 1 ? 4 : 2)}${t.usdPartial ? '+' : ''}`, hint: hint(costUnknown ? '暂无费用估算' : t.usdPartial ? 'USD · 估算不完整' : 'USD') },
  ];
}
