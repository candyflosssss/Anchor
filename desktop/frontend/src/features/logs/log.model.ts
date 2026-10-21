import type { CostAdjustment, KnownUsage, ReferenceCost } from '../../desktop/contract.generated';
import { adjustmentValue, costMultiplier, referenceUSD } from './log-reference-cost.model';
import { accelerationSummary, tierSnapshot } from './log-service-tier.model';
import type { Query } from '../../desktop/contract.generated';
export type LogDimension = 'agent' | 'binding' | 'project' | 'session';
export const dimensionLabels: Record<LogDimension, string> = { agent: 'Agent', binding: 'Agent 绑定', project: '项目', session: '会话' };
export type LogStatus = 'succeeded' | 'failed' | 'cancelled' | 'interrupted' | 'pending';
export type LogSource = 'proxy' | 'probe';
export interface LogTimeRange { since: number; before: number }
export function logTimeRange(window: string, range: LogTimeRange | null = null, now = Date.now()): LogTimeRange {
  return range ?? { since: now - Number(window) * 60000, before: now };
}
export interface LogAttempt {
  costAdjustment?: CostAdjustment | null;
  referenceUSD?: string | null;
  number: number; provider: string; key: string; model: string; protocol: string;
  http: number | null; input?: number | null; output?: number | null; elapsed: number; outcome: string; code?: string; note: string;
}
export interface RequestLog {
  knownUsage?: KnownUsage | null;
  referenceCost?: ReferenceCost | null;
  agent?: string; binding?: string; project?: string; session?: string;
  id: string; ageMinutes: number; time: string; status: LogStatus; source: LogSource;
  model: string; upstreamModel?: string; attemptCount?: number; attemptsTruncated?: boolean; protocol: string; elapsed: number | null; http: number | null;
  provider: string; key: string; revision: number; delivery: string;
  summary: string; stage: string; code?: string; input: number | null; output: number | null;
  reasoningEffort?: string | null; stream?: boolean; generationStatus?: string;
  metrics?: { outputGenerationMs?: number | null; nonReasoningOutputTokens?: number | null; outputIntervalComplete?: boolean; timeToFirstOutputMs?: number | null; timeToFirstTextMs: number | null; generationMs: number | null; outputTextTokens: number | null;
    textIntervalComplete: boolean; cacheReadTokens: number | null; reasoningTokens: number | null };
  attempts: LogAttempt[]; evidence: Record<string, string | number | boolean>;
}
export interface SystemLog {
  id: string; ageMinutes: number; time: string; level: 'info' | 'warn' | 'error';
  source: 'core' | 'desktop'; event: string; message: string; requestId?: string;
}
export interface LogFilters { agent?: string | null; binding?: string | null; project?: string | null; session?: string | null; query: string; status: string; source: string; provider: string; window: string; model: string; key: string; http: string; code: string; stage: string }
export const initialFilters: LogFilters = { agent: null, binding: null, project: null, session: null, query: '', status: 'all', source: 'all', provider: 'all', window: '60', model: '', key: '', http: '', code: '', stage: '' };
// null/undefined means all; an empty string is a deliberate unknown filter.
export function identityQuery(filters: LogFilters): Pick<Query, LogDimension> {
  return Object.fromEntries((Object.keys(dimensionLabels) as LogDimension[]).filter(key => filters[key] != null).map(key => [key, filters[key]]));
}
export const statusLabels: Record<LogStatus, string> = { succeeded: '成功', failed: '失败', cancelled: '已取消', interrupted: '已中断', pending: '进行中' };
export const sourceLabels: Record<LogSource, string> = { proxy: '代理请求', probe: '推理测试' };
export function encryptedContextRecoveryLabel(row: RequestLog): string | null {
  const outcome = row.evidence.encrypted_context_recovery;
  switch (outcome) {
    case 'succeeded': return '已清理加密上下文 · 恢复成功';
    case 'failed': return '已清理加密上下文 · 重试失败';
    case 'cancelled': return '已清理加密上下文 · 重试已取消';
    case 'pending': return `已清理加密上下文 · ${row.status === 'pending' ? '正在重试' : '结果未确认'}`;
    default: return null;
  }
}
export function filterRequests(rows: RequestLog[], filters: LogFilters): RequestLog[] {
  const query = filters.query.trim().toLowerCase();
  return rows.filter(row => row.ageMinutes <= Number(filters.window)
    && (Object.keys(dimensionLabels) as LogDimension[]).every(key => filters[key] == null || (row[key] ?? '') === filters[key])
    && (filters.status === 'all' || row.status === filters.status)
    && (filters.source === 'all' || row.source === filters.source)
    && (filters.provider === 'all' || row.attempts.some(attempt => attempt.provider === filters.provider))
    && [row.id, row.model, row.provider, row.key, row.code ?? '', row.agent ?? '', row.binding ?? '', row.project ?? '', row.session ?? '', ...row.attempts.map(attempt => `${attempt.provider} ${attempt.model} ${attempt.key}`)].join(' ').toLowerCase().includes(query));
}
export function safeSummary(row: RequestLog): string {
  const a = row.referenceCost?.adjustment;
  const supplier = !row.referenceCost ? '—' : !a ? '未记录' : costMultiplier(a.supplier_multiplier, a.supplier_mixed);
  const group = !row.referenceCost ? '—' : !a ? '未记录' : costMultiplier(a.group_multiplier, a.group_mixed);
  return ['Anchor 日志', `request_id: ${row.id}`, `结果: ${statusLabels[row.status]} · ${row.summary}`,
    ...(Object.keys(dimensionLabels) as LogDimension[]).map(key => `${dimensionLabels[key]}: ${row[key] || '未识别'}`),
    `原始参考费用: ${row.referenceCost && row.referenceCost.usd == null ? '未知' : referenceUSD(row.referenceCost?.usd)}`, `供应商倍率: ${supplier}`, `分组倍率: ${group}`, `最终参考费用: ${row.referenceCost ? adjustmentValue(row.referenceCost.adjustment) : '—'}`, `来源: ${sourceLabels[row.source]}`, `模型: ${row.model}`, `供应商 / Key: ${row.provider} / ${row.key}`,
    `请求档位: ${serviceTierLabel(row.evidence.requested_service_tier, true)} · 原始值 ${tierSnapshot(row.evidence, 'requested').raw}`,
    `发送档位（最后候选）: ${tierSnapshot(row.evidence, 'sent').raw}`,
    `加速状态（上游报告）: ${accelerationSummary(row.evidence).label} · ${accelerationSummary(row.evidence).tier.label}`,
    ...(row.evidence.mapping_service_tier ? [`Anchor 固定档位: ${serviceTierLabel(row.evidence.mapping_service_tier)}`] : []),
    `上游返回档位（最后候选）: ${serviceTierLabel(row.evidence.upstream_service_tier)} · 原始值 ${tierSnapshot(row.evidence, 'upstream').raw}`,
    `上游 HTTP: ${row.http ?? '未收到'}`, `阶段: ${row.stage}`, `错误代码: ${row.code ?? '无'}`,
    `交付: ${row.delivery}`, `耗时: ${row.elapsed === null ? '未知' : `${row.elapsed} ms`}`,
    ...Object.entries(row.evidence).map(([key, value]) => `${key}: ${String(value)}`)].join('\n');
}

// Render only known metadata; a requested tier is never used as the actual tier.
export function serviceTierLabel(value: unknown, requested = false): string {
  switch (value) {
    case 'priority': case 'fast': return 'Fast';
    case 'ultrafast': return 'Ultrafast';
    case 'default': return 'Standard';
    case 'auto': return 'Auto（自动）';
    case 'flex': return 'Flex';
    case 'scale': return 'Scale';
    case 'unspecified': return requested ? '未指定（由上游决定）' : '未知 / 未返回';
    default: return '未知 / 未返回';
  }
}
