import type { RequestLog } from './log.model';

export const columnGroups = ['基础信息', '模型与路由', '速度与用量', '结果与诊断'] as const;
type ColumnGroup = typeof columnGroups[number];
interface ColumnDefinition { id: string; label: string; group: ColumnGroup; description: string; width: number }
export const logColumns = [
  { id: 'time', label: '时间', group: '基础信息', description: '请求发生时间与相对时间。', width: 120 },
  { id: 'status', label: '结果与交付', group: '基础信息', description: 'Anchor 结果、上游 HTTP、错误码、生成状态与客户端交付；这些状态彼此独立。', width: 160 },
  { id: 'requestId', label: '请求 ID', group: '基础信息', description: '关联请求明细和系统事件的稳定标识。', width: 175 },
  { id: 'source', label: '来源', group: '基础信息', description: '代理请求或主动推理测试。', width: 105 },
  { id: 'model', label: '模型', group: '模型与路由', description: '公开模型、实际模型与请求设置的思考深度；不从模型名推断深度。', width: 185 },
  { id: 'provider', label: '供应商 / Key', group: '模型与路由', description: '最后一次尝试的供应商与 Key 安全标签；不显示密钥本体。', width: 130 },
  { id: 'protocol', label: '请求方式', group: '模型与路由', description: '请求与上游协议路径，以及是否流式返回。', width: 170 },
  { id: 'revision', label: '配置版本', group: '模型与路由', description: '请求当时的配置 revision 快照。', width: 105 },
  { id: 'timing', label: '耗时与速度', group: '速度与用量', description: '总耗时、首次有效输出时间、输出耗时与输出 TPS。输出含正文和工具调用参数，不含思考、角色或心跳；总耗时含重试等待。输出 TPS 为非思考输出 Token / 首次有效输出到完成的完整流式区间，区间可能包含交错思考或等待，并非纯解码速度。', width: 230 },
  { id: 'tokens', label: 'Token 用量', group: '速度与用量', description: '输入、输出、缓存命中、思考与输入输出合计。缓存和思考不重复累加，未报告不补零；多次尝试仅为已知小计。', width: 240 },
  { id: 'attempts', label: '尝试次数', group: '结果与诊断', description: '本次根请求实际派发的上游尝试次数，允许为零。', width: 100 },
  { id: 'diagnostics', label: '错误与阶段', group: '结果与诊断', description: '稳定错误码与请求失败 / 结束阶段；未记录错误码时显示“—”。', width: 190 },
] as const satisfies readonly ColumnDefinition[];
export type LogColumnId = typeof logColumns[number]['id'];
export const columnPresets = {
  common: { label: '常用', ids: ['time', 'status', 'model', 'provider', 'timing', 'tokens'] },
  performance: { label: '性能', ids: ['time', 'model', 'provider', 'protocol', 'timing', 'tokens'] },
  debugging: { label: '排障', ids: ['time', 'status', 'requestId', 'provider', 'diagnostics', 'attempts'] },
} satisfies Record<string, { label: string; ids: LogColumnId[] }>;
export const defaultColumns: LogColumnId[] = [...columnPresets.common.ids];
// Keep the existing key and migrate individual fields into blocks in their first selected position.
export const columnStorageKey = 'anchor.ui.logs.columns.v1';
const legacyColumns: Record<string, LogColumnId> = {
  upstreamModel: 'model', reasoningEffort: 'model', key: 'provider', stream: 'protocol',
  ttft: 'timing', elapsed: 'timing', generationMs: 'timing', tps: 'timing',
  input: 'tokens', output: 'tokens', totalTokens: 'tokens', cacheReadTokens: 'tokens', reasoningTokens: 'tokens',
  http: 'status', generationStatus: 'status', delivery: 'status', errorCode: 'diagnostics', stage: 'diagnostics',
};
export function normalizeColumns(value: unknown): LogColumnId[] {
  if (!Array.isArray(value)) return [...defaultColumns];
  const known = new Set<string>(logColumns.map(column => column.id));
  const ids: LogColumnId[] = [];
  for (const entry of value) {
    if (typeof entry !== 'string') continue;
    const id = known.has(entry) ? entry as LogColumnId : Object.hasOwn(legacyColumns, entry) ? legacyColumns[entry] : undefined;
    if (id && !ids.includes(id)) ids.push(id);
  }
  return ids.length ? ids : [...defaultColumns];
}
export function moveColumn(ids: LogColumnId[], id: LogColumnId, direction: -1 | 1): LogColumnId[] {
  const index = ids.indexOf(id), next = index + direction;
  if (index < 0 || next < 0 || next >= ids.length) return ids;
  const reordered = [...ids];
  [reordered[index], reordered[next]] = [reordered[next], reordered[index]];
  return reordered;
}
export function outputTps(row: RequestLog): number | null {
  const metrics = row.metrics;
  if (!row.stream || metrics?.outputIntervalComplete !== true || metrics.nonReasoningOutputTokens == null || metrics.outputGenerationMs == null
    || !Number.isFinite(metrics.nonReasoningOutputTokens) || !Number.isFinite(metrics.outputGenerationMs)
    || metrics.nonReasoningOutputTokens <= 0 || metrics.outputGenerationMs <= 0) return null;
  return metrics.nonReasoningOutputTokens / (metrics.outputGenerationMs / 1000);
}
