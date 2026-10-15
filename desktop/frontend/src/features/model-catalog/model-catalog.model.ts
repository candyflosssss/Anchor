import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { CatalogCapabilities, ContextPolicy, CatalogEstimate, CatalogEstimateInput, CatalogLimits, CatalogModel, CatalogPricing, CatalogRates, CatalogUsage, DesktopBridge, ModelCatalog, ModelCatalogQuery, ReasoningOption } from '../../desktop/contract.generated';

const bad = (): never => { throw new DesktopProblem('read_error'); };
const record = (v: unknown): Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : bad();
const string = (v: unknown): string => typeof v === 'string' ? v : bad();
const integer = (v: unknown, min = 0): number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min ? v : bad();
const boolean = (v: unknown): boolean => typeof v === 'boolean' ? v : bad();
const strings = (v: unknown): string[] => Array.isArray(v) ? v.map(string) : bad();
const nullable = <T>(v: unknown, check: (v: unknown) => T): T | null => v === null ? null : check(v);
const money = (v: unknown): string => {
  const text = string(v);
  if (text.length > 64 || !/^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$/.test(text) || !Number.isFinite(Number(text)) || Number(text) < 0) return bad();
  return text;
};
function rates(v: unknown): CatalogRates {
  const r = record(v);
  return { input: nullable(r.input, money), output: nullable(r.output, money), cache_read: nullable(r.cache_read, money), cache_write: nullable(r.cache_write, money), reasoning: nullable(r.reasoning, money), input_audio: nullable(r.input_audio, money), output_audio: nullable(r.output_audio, money) };
}
function pricing(v: unknown): CatalogPricing {
  const p = record(v);
  if (p.currency !== 'USD' || p.unit !== 'per_million_tokens' || !Array.isArray(p.tiers) || p.tiers.length > 32) return bad();
  const tiers = p.tiers.map(item => { const t = record(item); return { above_tokens: integer(t.above_tokens), rates: rates(t.rates) }; });
  if (tiers.some((t, i) => i > 0 && t.above_tokens <= tiers[i - 1].above_tokens)) return bad();
  return { currency: p.currency, unit: p.unit, rates: rates(p.rates), tiers };
}
function reasoning(v: unknown): ReasoningOption {
  const r = record(v);
  if (!Array.isArray(r.values) || r.values.length > 16) return bad();
  const values = r.values.map(v => nullable(v, string));
  const min = nullable(r.min, v => integer(v, -1)), max = nullable(r.max, integer);
  if (r.type === 'toggle') { if (values.length || min !== null || max !== null) return bad(); }
  else if (r.type === 'effort') {
    if (!values.length || min !== null || max !== null || values.some(v => v !== null && !['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'default'].includes(v))) return bad();
  } else if (r.type === 'budget_tokens') { if (values.length || (min !== null && max !== null && min > max)) return bad(); }
  else return bad();
  return { type: r.type, values, min, max };
}
export function checkedContextPolicy(v: unknown): ContextPolicy | null {
  // Old cores/snapshots did not carry advice. Missing is unknown, not zero.
  if (v === undefined || v === null) return null;
  const p = record(v);
  const model_context = nullable(p.model_context, v => integer(v, 1));
  const recommended_context = nullable(p.recommended_context, v => integer(v, 1));
  const price_threshold = nullable(p.price_threshold, v => integer(v, 1));
  if (recommended_context !== null && ((model_context !== null && recommended_context > model_context) || (price_threshold !== null && recommended_context > price_threshold))) return bad();
  return { model_context, recommended_context, price_threshold };
}
function model(v: unknown): CatalogModel {
  const m = record(v), c = record(m.capabilities), l = record(m.limits);
  const capabilities: CatalogCapabilities = { input_modalities: strings(c.input_modalities), output_modalities: strings(c.output_modalities), vision: nullable(c.vision, boolean), attachment: nullable(c.attachment, boolean), tool_call: nullable(c.tool_call, boolean), reasoning: nullable(c.reasoning, boolean), structured_output: nullable(c.structured_output, boolean), temperature: nullable(c.temperature, boolean) };
  const limits: CatalogLimits = { context: nullable(l.context, v => integer(v, 1)), input: nullable(l.input, v => integer(v, 1)), output: nullable(l.output, v => integer(v, 1)) };
  if (!Array.isArray(m.reasoning_options) || m.reasoning_options.length > 16) return bad();
  return { context_policy: checkedContextPolicy(m.context_policy), provider_id: string(m.provider_id), id: string(m.id), canonical_model_id: string(m.canonical_model_id), name: string(m.name), source_updated_at: string(m.source_updated_at), capabilities, limits, reasoning_options: m.reasoning_options.map(reasoning), pricing: nullable(m.pricing, pricing) };
}
export function checkedModelCatalog(v: unknown): ModelCatalog {
  const c = record(v);
  if (c.source !== 'models.dev' || typeof c.version !== 'string' || !/^[a-f0-9]{64}$/.test(c.version) || !Array.isArray(c.providers) || c.providers.length > 1024 || !Array.isArray(c.models) || c.models.length > 100) return bad();
  const stale = boolean(c.stale), failure = string(c.failure);
  if ((stale && failure !== 'refresh_failed') || (!stale && failure !== '')) return bad();
  const fetched = integer(c.fetched_at), expires = integer(c.expires_at);
  if (expires < fetched) return bad();
  const providers = c.providers.map(item => { const p = record(item); return { id: string(p.id), name: string(p.name), model_count: integer(p.model_count) }; });
  const models = c.models.map(model), total = integer(c.total), truncated = boolean(c.truncated);
  if (total < providers.length + models.length || (providers.length > 0 && models.length > 0) || (truncated && total <= models.length)) return bad();
  return { source: c.source, fetched_at: fetched, expires_at: expires, version: c.version, stale, failure, providers, models, total, truncated };
}
function usage(v: unknown): CatalogUsage {
  const u = record(v);
  const out = { input_tokens: nullable(u.input_tokens, integer), output_tokens: nullable(u.output_tokens, integer), cache_read_tokens: nullable(u.cache_read_tokens, integer), cache_write_tokens: nullable(u.cache_write_tokens, integer), reasoning_tokens: nullable(u.reasoning_tokens, integer) };
  if (out.input_tokens !== null && (out.cache_read_tokens ?? 0) + (out.cache_write_tokens ?? 0) > out.input_tokens) return bad();
  if (out.output_tokens !== null && out.reasoning_tokens !== null && out.reasoning_tokens > out.output_tokens) return bad();
  return out;
}
export function checkedCatalogEstimate(v: unknown): CatalogEstimate {
  const e = record(v);
  if (e.basis !== 'text_tokens' || e.currency !== 'USD') return bad();
  const usd = nullable(e.usd, money), reason = string(e.reason);
  if ((usd !== null && reason !== '') || (usd === null && !['missing_price', 'missing_usage', 'missing_cache_usage', 'missing_reasoning_usage', 'missing_input_price', 'missing_output_price', 'missing_cache_read_price', 'missing_cache_write_price', 'missing_reasoning_price'].includes(reason))) return bad();
  const catalog = checkedModelCatalog(e.catalog);
  if (catalog.models.length !== 1) return bad();
  return { catalog, usage: usage(e.usage), basis: e.basis, currency: e.currency, usd, input_usd: nullable(e.input_usd, money), output_usd: nullable(e.output_usd, money), cache_read_usd: nullable(e.cache_read_usd, money), cache_write_usd: nullable(e.cache_write_usd, money), reasoning_usd: nullable(e.reasoning_usd, money), above_tokens: nullable(e.above_tokens, integer), reason };
}
export function createModelCatalogService(getBridge: () => Pick<DesktopBridge, 'ModelCatalog' | 'EstimateModelCost'> = bridge) {
  return {
    query: async (input: ModelCatalogQuery, signal?: AbortSignal): Promise<ModelCatalog> => {
      signal?.throwIfAborted();
      const api = getBridge();
      if (typeof api.ModelCatalog !== 'function') throw new DesktopProblem('unsupported');
      const result = await call(() => api.ModelCatalog(input));
      signal?.throwIfAborted();
      const catalog = checkedModelCatalog(result);
      if (input.provider_id && catalog.models.some(m => m.provider_id !== input.provider_id || (input.model_id && m.id !== input.model_id))) return bad();
      if (input.model_id && catalog.models.length !== 1) return bad();
      return catalog;
    },
    estimate: async (input: CatalogEstimateInput, signal?: AbortSignal): Promise<CatalogEstimate> => {
      signal?.throwIfAborted();
      const api = getBridge();
      if (typeof api.EstimateModelCost !== 'function') throw new DesktopProblem('unsupported');
      const result = await call(() => api.EstimateModelCost(input));
      signal?.throwIfAborted();
      const estimate = checkedCatalogEstimate(result), m = estimate.catalog.models[0];
      if (m.provider_id !== input.query.provider_id || m.id !== input.query.model_id || Object.entries(input.usage).some(([key, value]) => estimate.usage[key as keyof CatalogUsage] !== value)) return bad();
      return estimate;
    },
  };
}
export const modelCatalogService = createModelCatalogService();
