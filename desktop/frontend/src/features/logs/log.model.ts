export type LogStatus = 'succeeded' | 'failed' | 'cancelled' | 'interrupted' | 'pending';
export type LogSource = 'proxy' | 'probe';
export interface LogAttempt {
  number: number; provider: string; key: string; model: string; protocol: string;
  http: number | null; elapsed: number; outcome: string; code?: string; note: string;
}
export interface RequestLog {
  id: string; ageMinutes: number; time: string; status: LogStatus; source: LogSource;
  model: string; protocol: string; elapsed: number | null; http: number | null;
  provider: string; key: string; revision: number; delivery: string;
  summary: string; stage: string; code?: string; input: number | null; output: number | null;
  attempts: LogAttempt[]; evidence: Record<string, string | number | boolean>;
}
export interface SystemLog {
  id: string; ageMinutes: number; time: string; level: 'info' | 'warn' | 'error';
  source: 'core' | 'desktop'; event: string; message: string; requestId?: string;
}
export interface LogFilters { query: string; status: string; source: string; provider: string; window: string }
export const initialFilters: LogFilters = { query: '', status: 'all', source: 'all', provider: 'all', window: '60' };
export const statusLabels: Record<LogStatus, string> = { succeeded: '成功', failed: '失败', cancelled: '已取消', interrupted: '已中断', pending: '进行中' };
export const sourceLabels: Record<LogSource, string> = { proxy: '代理请求', probe: '推理测试' };
export function filterRequests(rows: RequestLog[], filters: LogFilters): RequestLog[] {
  const query = filters.query.trim().toLowerCase();
  return rows.filter(row => row.ageMinutes <= Number(filters.window)
    && (filters.status === 'all' || row.status === filters.status)
    && (filters.source === 'all' || row.source === filters.source)
    && (filters.provider === 'all' || row.attempts.some(attempt => attempt.provider === filters.provider))
    && [row.id, row.model, row.provider, row.key, row.code ?? '', ...row.attempts.map(attempt => `${attempt.provider} ${attempt.model} ${attempt.key}`)].join(' ').toLowerCase().includes(query));
}
export function safeSummary(row: RequestLog): string {
  return ['Anchor 日志设计预览 · 虚拟数据', `request_id: ${row.id}`, `结果: ${statusLabels[row.status]} · ${row.summary}`,
    `来源: ${sourceLabels[row.source]}`, `模型: ${row.model}`, `供应商 / Key: ${row.provider} / ${row.key}`,
    `上游 HTTP: ${row.http ?? '未收到'}`, `阶段: ${row.stage}`, `错误代码: ${row.code ?? '无'}`,
    `交付: ${row.delivery}`, `耗时: ${row.elapsed === null ? '未知' : `${row.elapsed} ms`}`,
    ...Object.entries(row.evidence).map(([key, value]) => `${key}: ${String(value)}`)].join('\n');
}
