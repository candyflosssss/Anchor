import type { KeyStatus } from '../../desktop/contract.generated';
import type { ProviderSummary } from '../providers/provider-summary.model';
import type { ModelRouting, ModelRoutingState, RouteCandidate } from '../../desktop/contract.generated';
import { ProviderProblem } from '../providers/provider.model';
export type { ModelRouting, ModelRoutingState, RouteCandidate };
export const routingRules = {
  'round-robin': { label: '按倍率加权轮询', hint: '按供应商倍率 × Key 缓存分组倍率的倒数分配请求；零倍率优先，未知仅作回退，全部未知时等权；不可用来源自动跳过。' },
  priority: { label: '优先级顺序', hint: '按保存的候选顺序请求，失败后切换到下一个可用来源。' },
  'cache-priority': { label: '缓存优先', hint: '同会话保持成功渠道；新会话按保存顺序选择。上游缓存命中由平台决定。' },
  'lowest-price': { label: '低价优先', hint: '按供应商倍率 × Key 缓存分组倍率从低到高选择；未知倍率仅作回退。相对倍率不等于实际单价。' },
  'lowest-latency': { label: '低延迟优先', hint: '先按保存顺序获取各来源的成功观测，再优先选择近期首字延迟更低的来源；无首字时使用总耗时。观测保留30分钟，重启后重新学习。' },
} as const;
export const validRoutingRule = (v: string) => Object.hasOwn(routingRules, v);
export const ruleLabel = (v: string) => routingRules[v as keyof typeof routingRules]?.label ?? v;
export const routeID = (v: RouteCandidate) => JSON.stringify([v.provider_id, v.model]);
export function effectiveRouteMultiplier(candidate: { cost_multiplier: number; multiplier: number; availability: { state: string }; key_ids: string[]; keys: { id: string; enabled: boolean; state: string; effective_multiplier: number | null; rate: { multiplier: number | null } | null }[] }): number | null {
  if (candidate.availability.state !== 'available') return null;
  const scoped = candidate.key_ids.length ? candidate.key_ids : candidate.keys.length ? candidate.keys.map(k => k.id) : ['anonymous'];
  const values = scoped.flatMap(id => {
    if (id === 'anonymous' && !candidate.keys.length) return [candidate.cost_multiplier * candidate.multiplier];
    const key = candidate.keys.find(k => k.id === id);
    if (!key || !key.enabled || !['ready', 'degraded'].includes(key.state)) return [];
    return key.effective_multiplier == null ? [] : [key.effective_multiplier * candidate.multiplier];
  }).filter(value => Number.isFinite(value) && value >= 0);
  return values.length ? Math.min(...values) : null;
}
export function priceOrder<T extends { provider_id: string; model: string }>(items: T[], multiplier: (item: T) => number | null): T[] {
  return items.map((item, index) => ({ item, index, value: multiplier(item) })).sort((a, b) => {
    if (a.value == null && b.value != null) return 1;
    if (a.value != null && b.value == null) return -1;
    return (a.value ?? 0) - (b.value ?? 0) || a.index - b.index;
  }).map(v => v.item);
}
export function checkedRouting(v: ModelRoutingState, input: ModelRouting): ModelRoutingState {
  if (!v || v.model_id !== input.model_id || v.revision !== input.expected_revision + 1 || v.strategy !== input.strategy || !Array.isArray(v.order) || v.order.length !== input.order.length || v.order.some((c, i) => c.provider_id !== input.order[i].provider_id || c.model !== input.order[i].model)) throw new ProviderProblem('unknown');
  return { model_id: v.model_id, revision: v.revision, strategy: v.strategy, order: v.order.map(c => ({ provider_id: c.provider_id, model: c.model })) };
}
export function moveRoute<T>(values: T[], from: number, to: number): T[] {
  if (from < 0 || to < 0 || from >= values.length || to >= values.length) return values;
  const next = [...values]; const [item] = next.splice(from, 1); next.splice(to, 0, item); return next;
}

/** Rate refreshes update billing fields only; scheduling still uses the workspace snapshot. */
export function withMappingRates(summaries: ProviderSummary[], keys: Record<string, KeyStatus>): ProviderSummary[] {
  return summaries.map(summary => !summary.workspace ? summary : { ...summary, workspace: { ...summary.workspace, keys: summary.workspace.keys.map(key => {
    const refreshed = keys[JSON.stringify([summary.provider.id, key.id])];
    return refreshed ? { ...key, rate: refreshed.rate, effective_multiplier: refreshed.effective_multiplier } : key;
  }) } });
}
