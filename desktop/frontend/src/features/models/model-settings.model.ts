import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { DesktopBridge, ModelCatalog, ModelConfiguration, ModelSettingsState, SaveModelSettings } from '../../desktop/contract.generated';
import { checkedModelCatalog } from '../model-catalog/model-catalog.model';

const bad = (): never => { throw new DesktopProblem('read_error'); };
const obj = (v: unknown): Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : bad();
const bool = (v: unknown): boolean | null => v === null || typeof v === 'boolean' ? v : bad();
const count = (v: unknown, min = 1): number | null => v === null ? null : typeof v === 'number' && Number.isSafeInteger(v) && v >= min ? v : bad();
const strings = (v: unknown): string[] => Array.isArray(v) && v.length <= 16 && v.every(x => typeof x === 'string' && x.length > 0 && x.length <= 128) ? [...v] : bad();
export function checkedConfiguration(v: unknown): ModelConfiguration {
  const c = obj(v), l = obj(c.limits), cap = obj(c.capabilities);
  const limits = { context: count(l.context), input: count(l.input), output: count(l.output) };
  const capabilities = { input_modalities: strings(cap.input_modalities), output_modalities: strings(cap.output_modalities), vision: bool(cap.vision), attachment: bool(cap.attachment), tool_call: bool(cap.tool_call), reasoning: bool(cap.reasoning), structured_output: bool(cap.structured_output), temperature: bool(cap.temperature) };
  if (!Array.isArray(c.supported_reasoning) || c.supported_reasoning.length > 3) return bad();
  const supported_reasoning = c.supported_reasoning.map(item => {
    const r = obj(item);
    if (!['effort', 'toggle', 'budget_tokens'].includes(String(r.type)) || !Array.isArray(r.values) || r.values.length > 32 || r.values.some(x => x !== null && (typeof x !== 'string' || !x.length || x.length > 128))) return bad();
    const option = { type: String(r.type), values: [...r.values] as (string | null)[], min: count(r.min, -1), max: count(r.max, 0) };
    if (option.type === 'effort' && (!option.values.length || option.min !== null || option.max !== null)) return bad();
    if (option.type !== 'effort' && option.values.length) return bad();
    if (option.type === 'toggle' && (option.min !== null || option.max !== null)) return bad();
    if (option.min !== null && option.max !== null && option.min > option.max) return bad();
    return option;
  });
  let default_reasoning: ModelConfiguration['default_reasoning'] = null;
  if (c.default_reasoning !== null) {
    const d = obj(c.default_reasoning);
    if (typeof d.type !== 'string' || (d.value !== null && typeof d.value !== 'string')) return bad();
    default_reasoning = { type: d.type, value: d.value, budget_tokens: count(d.budget_tokens, -1), enabled: bool(d.enabled) };
    const o = supported_reasoning.find(o => o.type === d.type);
    if (!o) return bad();
    if (o.type === 'effort' && (!o.values.includes(default_reasoning.value) || default_reasoning.budget_tokens !== null || default_reasoning.enabled !== null)) return bad();
    if (o.type === 'toggle' && (default_reasoning.enabled === null || default_reasoning.value !== null || default_reasoning.budget_tokens !== null)) return bad();
    if (o.type === 'budget_tokens' && (default_reasoning.budget_tokens === null || default_reasoning.value !== null || default_reasoning.enabled !== null || (o.min !== null && default_reasoning.budget_tokens < o.min) || (o.max !== null && default_reasoning.budget_tokens > o.max))) return bad();
  }
  if (new Set(supported_reasoning.map(o => o.type)).size !== supported_reasoning.length) return bad();
  if ([capabilities.input_modalities, capabilities.output_modalities].some(v => new Set(v).size !== v.length)) return bad();
  if (capabilities.vision !== null && capabilities.input_modalities.length && capabilities.vision !== capabilities.input_modalities.includes('image')) return bad();
  if (c.parallel_tool_calls === true && capabilities.tool_call === false) return bad();
  const output = count(c.default_output_tokens), temp = c.default_temperature;
  if (temp !== null && (typeof temp !== 'number' || !Number.isFinite(temp) || temp < 0 || temp > 2 || capabilities.temperature !== true)) return bad();
  if (limits.context !== null && [limits.input, limits.output, output].some(n => n !== null && n > limits.context!)) return bad();
  if (limits.output !== null && output !== null && output > limits.output) return bad();
  if (capabilities.reasoning === false && (supported_reasoning.length || default_reasoning !== null)) return bad();
  return { limits, capabilities, streaming: bool(c.streaming), parallel_tool_calls: bool(c.parallel_tool_calls), supported_reasoning, default_reasoning, default_output_tokens: output, default_temperature: temp };
}
export function checkedModelSettings(value: unknown, id: string): ModelSettingsState {
  const s = obj(value);
  if (s.model_id !== id || typeof s.display_name !== 'string' || s.display_name.length > 256) return bad();
  const revision = count(s.revision), candidate_count = count(s.candidate_count);
  if (revision === null || candidate_count === null || candidate_count > 16) return bad();
  const reference = s.reference === null ? null : checkedModelCatalog(s.reference);
  if (reference && reference.models.length !== 1) return bad();
  return { model_id: id, revision, candidate_count, display_name: s.display_name, configuration: checkedConfiguration(s.configuration), reference };
}
// Import is explicit. Fill mode protects configured values; replacement mode
// resets defaults that may not exist in the new provider's reasoning range.
export function importConfiguration(old: ModelConfiguration, catalog: ModelCatalog, replace: boolean): ModelConfiguration {
  if (catalog.models.length !== 1) return bad();
  const m = catalog.models[0], caps = structuredClone(old.capabilities);
  for (const key of ['vision', 'attachment', 'tool_call', 'reasoning', 'structured_output', 'temperature'] as const) if (replace || caps[key] === null) caps[key] = m.capabilities[key];
  for (const key of ['input_modalities', 'output_modalities'] as const) if (replace || !caps[key].length) caps[key] = [...m.capabilities[key]];
  // A manual vision declaration is authoritative in fill mode.
  if (caps.vision !== null && caps.input_modalities.length) caps.input_modalities = caps.vision ? [...new Set([...caps.input_modalities, 'image'])] : caps.input_modalities.filter(v => v !== 'image');
  const limits = { ...old.limits };
  for (const key of ['context', 'input', 'output'] as const) if (replace || limits[key] === null) limits[key] = m.limits[key];
  return { ...structuredClone(old), limits, capabilities: caps, supported_reasoning: caps.reasoning === false ? [] : replace || !old.supported_reasoning.length ? structuredClone(m.reasoning_options) : structuredClone(old.supported_reasoning), default_reasoning: replace || caps.reasoning === false ? null : structuredClone(old.default_reasoning) };
}
export function createModelSettingsService(getBridge: () => Pick<DesktopBridge, 'ModelSettings' | 'SaveModelSettings'> = bridge) {
  return {
    read: async (id: string, signal: AbortSignal) => { signal.throwIfAborted(); const api = getBridge(); if (!api.ModelSettings) throw new DesktopProblem('unsupported'); const out = await call(() => api.ModelSettings({ model_id: id })); signal.throwIfAborted(); return checkedModelSettings(out, id); },
    save: async (input: SaveModelSettings, signal: AbortSignal) => { signal.throwIfAborted(); try { checkedConfiguration(input.configuration); } catch { throw new DesktopProblem('invalid'); } const api = getBridge(); if (!api.SaveModelSettings) throw new DesktopProblem('unsupported'); const out = await call(() => api.SaveModelSettings(input), true); signal.throwIfAborted(); let s: ModelSettingsState; try { s = checkedModelSettings(out, input.model_id); } catch { throw new DesktopProblem('unknown'); } if (s.revision !== input.expected_revision + 1) throw new DesktopProblem('unknown'); return s; },
  };
}
export const modelSettingsService = createModelSettingsService();
export const settingsError = (error: unknown): string => {
  const code = error instanceof DesktopProblem ? error.code : 'read_error';
  return code === 'conflict' ? '配置已被其他操作修改，请重新读取后编辑。' : code === 'unknown' ? '保存结果未知，请重新读取并核对；不会自动重试。' : code === 'invalid' ? '设置无效，请核对限制与默认思考值。' : code === 'unsupported' ? '当前桌面核心不支持模型设置，请升级后重试。' : '读取或保存失败，请检查连接与输入后重试。';
};
