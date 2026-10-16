import type { RequestLog } from './log.model';

type Evidence = RequestLog['evidence'];
export type TierKind = 'fast' | 'ultrafast' | 'standard' | 'auto' | 'other' | 'unknown' | 'none';
export interface TierSnapshot { raw: string; value?: string; kind: TierKind; label: string }
const tiers: Record<string, { kind: TierKind; label: string }> = {
  priority: { kind: 'fast', label: 'Fast' }, fast: { kind: 'fast', label: 'Fast' },
  ultrafast: { kind: 'ultrafast', label: 'Ultrafast' }, default: { kind: 'standard', label: 'Standard' },
  auto: { kind: 'auto', label: 'Auto' }, flex: { kind: 'other', label: 'Flex' }, scale: { kind: 'other', label: 'Scale' },
};
export function tierSnapshot(evidence: Evidence, side: 'requested' | 'sent' | 'upstream'): TierSnapshot {
  const normalized = evidence[`${side}_service_tier`];
  const raw = evidence[`${side}_service_tier_raw`];
  // JSON scalar text distinguishes missing from null, empty string and sentinel-looking strings.
  if (raw !== undefined) {
    if (typeof raw !== 'string' || raw.length > 256) return { raw: '未知 / 无效记录', kind: 'unknown', label: '无法确认' };
    try {
      const value: unknown = JSON.parse(raw);
      if (value === null || ['string', 'number', 'boolean'].includes(typeof value)) {
        const known = typeof value === 'string' && Object.hasOwn(tiers, value) ? tiers[value] : undefined;
        return { raw, value: typeof value === 'string' ? value : undefined, kind: known?.kind ?? 'unknown', label: known?.label ?? '未解析' };
      }
    } catch { /* Malformed evidence never proves a tier. */ }
    return { raw: '未知 / 无效记录', kind: 'unknown', label: '无法确认' };
  }
  // Historical known values are already observations, not inferred from model names or policy.
  if (typeof normalized === 'string' && Object.hasOwn(tiers, normalized)) {
    return { raw: JSON.stringify(normalized), value: normalized, ...tiers[normalized] };
  }
  if (normalized === 'not_sent' && side === 'sent') return { raw: '未发送', kind: 'none', label: '未派发' };
  if (normalized === 'unspecified' && side !== 'upstream') return { raw: '未指定', kind: 'none', label: '无档位参数' };
  if (normalized === 'unknown') return { raw: '未知 / 原值未保留', kind: 'unknown', label: '无法确认' };
  return { raw: side === 'upstream' && evidence.sent_service_tier !== undefined ? '未返回' : '未知 / 未记录', kind: 'unknown', label: '无法确认' };
}
export function accelerationSummary(evidence: Evidence): { tier: TierSnapshot; state: 'confirmed' | 'mismatch' | 'none' | 'unknown'; label: string } {
  const sent = tierSnapshot(evidence, 'sent'), tier = tierSnapshot(evidence, 'upstream');
  const requestedFast = sent.kind === 'fast' || sent.kind === 'ultrafast';
  if (tier.kind === 'fast' || tier.kind === 'ultrafast') {
    if (sent.kind === 'ultrafast' && tier.kind === 'fast') return { tier, state: 'mismatch', label: '仅确认 Fast' };
    return { tier, state: 'confirmed', label: requestedFast ? '已确认加速' : '上游报告加速' };
  }
  if (tier.kind === 'standard' || tier.kind === 'other') {
    return { tier, state: requestedFast ? 'mismatch' : 'none', label: requestedFast ? '未按请求加速' : '非 Fast 档位' };
  }
  return { tier, state: 'unknown', label: '无法确认加速' };
}
