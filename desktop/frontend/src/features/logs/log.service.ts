import { checkedStats } from './log-stats.model';
import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { Query, Page, Request, Event, Settings } from '../../desktop/contract.generated';
import { statusLabels, type RequestLog, type SystemLog, type LogStatus, type LogSource } from './log.model';
export type LogQuery = Query;
export type LogSettings = Settings;
export function requestLog(row: Request): RequestLog {
  if (!row || !Object.hasOwn(statusLabels, row.status) || !['proxy', 'probe'].includes(row.source) || !Array.isArray(row.attempts) || !row.metrics) throw new DesktopProblem('read_error');
  // Older stored records may arrive re-serialized with null/false new fields.
  // Fall back only to already-confirmed complete body-text metrics, never total usage.
  const legacyText = row.metrics.output_generation_ms == null && row.metrics.non_reasoning_output_tokens == null
    && !row.metrics.output_interval_complete && row.metrics.text_interval_complete && row.metrics.output_text_tokens != null;
  return { agent: row.agent, binding: row.binding, project: row.project, session: row.session, id: row.id, ageMinutes: Math.max(0, Math.floor((Date.now() - row.started_at_ms) / 60000)), time: new Date(row.started_at_ms).toLocaleString(),
    status: row.status as LogStatus, source: row.source as LogSource, model: row.model || '未知', upstreamModel: row.upstream_model || undefined,
    protocol: row.protocol, elapsed: row.elapsed_ms, http: row.http, provider: row.provider || '未知', key: row.key || '未知', revision: row.revision,
    delivery: row.delivery || '未知', summary: row.code === 'unexpected_html' ? '上游返回了 HTML 网页，而不是模型响应。请检查 API 请求地址（通常以 /v1 结尾）。' : row.code || statusLabels[row.status as LogStatus], stage: row.stage, code: row.code || undefined,
    input: row.input, output: row.output, reasoningEffort: row.reasoning_observed ? row.reasoning_effort : undefined,
    stream: row.stream ?? undefined, generationStatus: row.generation_status || undefined, attemptCount: row.attempt_count, attemptsTruncated: row.attempts_truncated,
    metrics: {
      outputGenerationMs: legacyText ? row.metrics.generation_ms : row.metrics.output_generation_ms,
      nonReasoningOutputTokens: legacyText ? row.metrics.output_text_tokens : row.metrics.non_reasoning_output_tokens,
      outputIntervalComplete: legacyText || row.metrics.output_interval_complete === true,
      timeToFirstOutputMs: row.metrics.time_to_first_output_ms ?? row.metrics.time_to_first_text_ms, timeToFirstTextMs: row.metrics.time_to_first_text_ms, generationMs: row.metrics.generation_ms, outputTextTokens: row.metrics.output_text_tokens,
      textIntervalComplete: row.metrics.text_interval_complete, cacheReadTokens: row.metrics.cache_read_tokens, reasoningTokens: row.metrics.reasoning_tokens },
    attempts: row.attempts.map(a => ({ number: a.number, provider: a.provider, key: a.key, model: a.model, protocol: a.protocol, http: a.http, elapsed: a.elapsed_ms,
      input: a.input, output: a.output, outcome: a.outcome, code: a.code || undefined, note: a.stage })), evidence: row.evidence ?? {} };
}
export function systemLog(row: Event): SystemLog {
  return { id: row.id, ageMinutes: Math.max(0, Math.floor((Date.now() - row.at_ms) / 60000)), time: new Date(row.at_ms).toLocaleString(), level: row.level as SystemLog['level'],
    source: row.source as SystemLog['source'], event: row.event, message: Object.entries(row.fields ?? {}).map(([k,v]) => `${k}=${v}`).join(' · '), requestId: row.request_id || undefined };
}
export const logService = {
  async suggestions(query: import('../../desktop/contract.generated').SuggestionsQuery) {
    const value = await call(() => bridge().LogSuggestions(query));
    if (!Array.isArray(value.items) || value.items.length > 100 || typeof value.has_more !== 'boolean' || value.items.some(item =>
      !item || typeof item.value !== 'string' || !item.value || !Number.isSafeInteger(item.count) || item.count < 1)) throw new DesktopProblem('read_error');
    return value;
  },
  stats: async (query: import('../../desktop/contract.generated').StatsQuery) => checkedStats(await call(() => bridge().LogStats(query))),
  async list(query: Query): Promise<Page> { const p = await call(() => bridge().Logs(query)); if (!Array.isArray(p.requests) || !Array.isArray(p.events) || !p.health) throw new DesktopProblem('read_error'); p.requests.forEach(requestLog); p.events.forEach(systemLog); return p; },
  detail: async (id: string) => requestLog(await call(() => bridge().LogDetail(id))),
  settings: () => call(() => bridge().LogSettings()),
  configure: (s: Settings) => call(() => bridge().ConfigureLogs(s), true),
  export: (q: Query) => call(() => bridge().ExportLogs(q)),
};
