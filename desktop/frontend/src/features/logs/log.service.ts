import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { Query, Page, Request, Event, Settings } from '../../desktop/contract.generated';
import { statusLabels, type RequestLog, type SystemLog, type LogStatus, type LogSource } from './log.model';
export type LogQuery = Query;
export type LogSettings = Settings;
export function requestLog(row: Request): RequestLog {
  if (!row || !Object.hasOwn(statusLabels, row.status) || !['proxy', 'probe'].includes(row.source) || !Array.isArray(row.attempts) || !row.metrics) throw new DesktopProblem('read_error');
  return { id: row.id, ageMinutes: Math.max(0, Math.floor((Date.now() - row.started_at_ms) / 60000)), time: new Date(row.started_at_ms).toLocaleString(),
    status: row.status as LogStatus, source: row.source as LogSource, model: row.model || '未知', upstreamModel: row.upstream_model || undefined,
    protocol: row.protocol, elapsed: row.elapsed_ms, http: row.http, provider: row.provider || '未知', key: row.key || '未知', revision: row.revision,
    delivery: row.delivery || '未知', summary: row.code === 'unexpected_html' ? '上游返回了 HTML 网页，而不是模型响应。请检查 API 请求地址（通常以 /v1 结尾）。' : row.code || statusLabels[row.status as LogStatus], stage: row.stage, code: row.code || undefined,
    input: row.input, output: row.output, reasoningEffort: row.reasoning_observed ? row.reasoning_effort : undefined,
    stream: row.stream ?? undefined, generationStatus: row.generation_status || undefined, attemptCount: row.attempt_count, attemptsTruncated: row.attempts_truncated,
    metrics: { timeToFirstTextMs: row.metrics.time_to_first_text_ms, generationMs: row.metrics.generation_ms, outputTextTokens: row.metrics.output_text_tokens,
      textIntervalComplete: row.metrics.text_interval_complete, cacheReadTokens: row.metrics.cache_read_tokens, reasoningTokens: row.metrics.reasoning_tokens },
    attempts: row.attempts.map(a => ({ number: a.number, provider: a.provider, key: a.key, model: a.model, protocol: a.protocol, http: a.http, elapsed: a.elapsed_ms,
      input: a.input, output: a.output, outcome: a.outcome, code: a.code || undefined, note: a.stage })), evidence: row.evidence ?? {} };
}
export function systemLog(row: Event): SystemLog {
  return { id: row.id, ageMinutes: Math.max(0, Math.floor((Date.now() - row.at_ms) / 60000)), time: new Date(row.at_ms).toLocaleString(), level: row.level as SystemLog['level'],
    source: row.source as SystemLog['source'], event: row.event, message: Object.entries(row.fields ?? {}).map(([k,v]) => `${k}=${v}`).join(' · '), requestId: row.request_id || undefined };
}
export const logService = {
  async list(query: Query): Promise<Page> { const p = await call(() => bridge().Logs(query)); if (!Array.isArray(p.requests) || !Array.isArray(p.events) || !p.health) throw new DesktopProblem('read_error'); p.requests.forEach(requestLog); p.events.forEach(systemLog); return p; },
  detail: async (id: string) => requestLog(await call(() => bridge().LogDetail(id))),
  settings: () => call(() => bridge().LogSettings()),
  configure: (s: Settings) => call(() => bridge().ConfigureLogs(s), true),
  export: (q: Query) => call(() => bridge().ExportLogs(q)),
};
