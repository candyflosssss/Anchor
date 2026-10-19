import type { CostAdjustment } from '../../desktop/contract.generated';
// Keep the decimal string so a small but nonzero reference cost never rounds to zero.
export function referenceUSD(value: string | null | undefined): string {
  if (value == null || !/^\d+(?:\.\d+)?$/.test(value)) return '—';
  const [whole, fraction = ''] = value.split('.');
  const decimals = fraction.replace(/0+$/, '');
  return `$${whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}${decimals ? `.${decimals}` : ''}`;
}

export function referenceCostReason(reason: string): string {
  switch (reason) {
    case 'missing_multiplier': return '供应商或分组倍率未观测，最终费用未知';
    case 'missing_usage': return '上游未报告完整用量';
    case 'missing_cache_usage': return '上游未报告完整缓存用量';
    case 'missing_reasoning_usage': return '缺少单独计价所需的思考用量';
    case 'invalid_usage': return '上游用量无效';
    default: return '参考单价不完整';
  }
}

export function costMultiplier(value: string | null | undefined, mixed?: boolean): string {
  return mixed ? '逐次' : value == null ? '未知' : `×${value}`;
}
export function adjustmentValue(adjustment: CostAdjustment | null | undefined): string {
  return adjustment == null ? '未记录' : adjustment.usd == null ? '未知' : referenceUSD(adjustment.usd);
}
