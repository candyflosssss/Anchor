import type { RequestLog } from './log.model';

export const columnGroups = ['基础信息', '模型与路由', '速度与用量', '结果与诊断'] as const;
type ColumnGroup = typeof columnGroups[number];
interface ColumnDefinition { id: string; label: string; group: ColumnGroup; description: string; width: number; numeric?: boolean }
export const logColumns = [
  { id: 'time', label: '时间', group: '基础信息', description: '请求发生时间；下方显示合成相对时间。', width: 110 },
  { id: 'status', label: '结果', group: '基础信息', description: 'Anchor 最终处理结果，与上游 HTTP 分开。', width: 100 },
  { id: 'requestId', label: '请求 ID', group: '基础信息', description: '关联请求明细和系统事件的稳定标识。', width: 175 },
  { id: 'source', label: '来源', group: '基础信息', description: '代理请求或主动推理测试。', width: 105 },
  { id: 'model', label: '模型名称', group: '模型与路由', description: '调用方使用的公开模型名。', width: 185 },
  { id: 'upstreamModel', label: '实际模型', group: '模型与路由', description: '最后一次上游尝试的模型名；全部尝试见详情。', width: 170 },
  { id: 'provider', label: '供应商', group: '模型与路由', description: '最后一次上游尝试的供应商。', width: 105 },
  { id: 'key', label: 'Key 标识', group: '模型与路由', description: '仅安全标签，不显示密钥本体。', width: 105 },
  { id: 'protocol', label: '协议', group: '模型与路由', description: '请求与上游使用的协议路径。', width: 170 },
  { id: 'reasoningEffort', label: '思考深度', group: '模型与路由', description: '请求设置的 reasoning effort；不从模型名或耗时推断。', width: 105 },
  { id: 'stream', label: '流式', group: '模型与路由', description: '这次请求是否使用流式返回。', width: 80 },
  { id: 'revision', label: '配置版本', group: '模型与路由', description: '请求当时的配置 revision 快照。', width: 105 },
  { id: 'ttft', label: '首字时间', group: '速度与用量', description: '根请求开始到首次向客户端写出正文的时间；不把思考或首个网络包当正文。', width: 110, numeric: true },
  { id: 'elapsed', label: '总耗时', group: '速度与用量', description: '根请求开始到结束的时间，包含重试和等待。', width: 110, numeric: true },
  { id: 'generationMs', label: '正文生成耗时', group: '速度与用量', description: '首个正文 Token 到正文生成结束的观测区间；非流式或区间不完整时未知。', width: 130, numeric: true },
  { id: 'tps', label: 'TPS', group: '速度与用量', description: '正文 Token ÷ 正文生成秒数；不包含思考 Token、首字等待或重试时间，区间完整且可确认时才计算。', width: 100, numeric: true },
  { id: 'input', label: '输入 Token', group: '速度与用量', description: '上游确认的输入用量；多次尝试仅展示已知小计。', width: 110, numeric: true },
  { id: 'output', label: '输出 Token', group: '速度与用量', description: '上游报告的输出用量，可能包含思考 Token；多次尝试仅展示已知小计。', width: 110, numeric: true },
  { id: 'totalTokens', label: '总 Token', group: '速度与用量', description: '已知输入与输出相加；任一缺失时未知，多次尝试仍是已知小计。', width: 110, numeric: true },
  { id: 'cacheReadTokens', label: '缓存命中 Token', group: '速度与用量', description: '上游明确报告的缓存读取用量；未报告不补零。', width: 145, numeric: true },
  { id: 'reasoningTokens', label: '思考 Token', group: '速度与用量', description: '上游明确报告的 reasoning 用量；不同于思考深度设置。', width: 115, numeric: true },
  { id: 'attempts', label: '尝试次数', group: '结果与诊断', description: '本次根请求实际派发的上游尝试次数，允许为零。', width: 100, numeric: true },
  { id: 'http', label: '上游 HTTP', group: '结果与诊断', description: '最后一次尝试的 HTTP 状态；未收到时显示“未收到”。', width: 110 },
  { id: 'errorCode', label: '错误码', group: '结果与诊断', description: '稳定错误码；没有记录错误码时显示“—”。', width: 190 },
  { id: 'stage', label: '失败 / 结束阶段', group: '结果与诊断', description: '请求结束时的处理阶段。', width: 175 },
  { id: 'generationStatus', label: '生成状态', group: '结果与诊断', description: '上游生成状态，与 HTTP 和 Anchor 校验结果独立。', width: 125 },
  { id: 'delivery', label: '客户端交付', group: '结果与诊断', description: '完整写出、部分输出、未输出或交付未知。', width: 155 },
] as const satisfies readonly ColumnDefinition[];
export type LogColumnId = typeof logColumns[number]['id'];
export const columnPresets = {
  common: { label: '常用', ids: ['time', 'status', 'model', 'provider', 'ttft', 'elapsed', 'tps', 'errorCode'] },
  performance: { label: '性能', ids: ['time', 'model', 'provider', 'reasoningEffort', 'ttft', 'elapsed', 'tps', 'input', 'output'] },
  debugging: { label: '排障', ids: ['time', 'status', 'requestId', 'provider', 'key', 'http', 'errorCode', 'stage', 'attempts'] },
} satisfies Record<string, { label: string; ids: LogColumnId[] }>;
export const defaultColumns: LogColumnId[] = [...columnPresets.common.ids];
export const columnStorageKey = 'anchor.ui.logs.columns.v1';
export function normalizeColumns(value: unknown): LogColumnId[] {
  if (!Array.isArray(value)) return [...defaultColumns];
  const known = new Set<string>(logColumns.map(column => column.id));
  const ids = [...new Set(value.filter((id): id is LogColumnId => typeof id === 'string' && known.has(id)))];
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
  if (!row.stream || metrics?.textIntervalComplete !== true || metrics.outputTextTokens == null || metrics.generationMs == null
    || !Number.isFinite(metrics.outputTextTokens) || !Number.isFinite(metrics.generationMs)
    || metrics.outputTextTokens <= 0 || metrics.generationMs <= 0) return null;
  return metrics.outputTextTokens / (metrics.generationMs / 1000);
}
