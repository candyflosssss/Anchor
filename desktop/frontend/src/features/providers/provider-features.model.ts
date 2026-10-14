import { isValidId } from '../../lib/identifier';
import { validRoutingRule } from '../models/model-routing.model';
import type { Action, Price, Probe, Workspace } from '../../desktop/contract.generated';
import { protocolName, ProviderProblem, type Provider } from './provider.model';
export type { Action, Price, Probe, Workspace };
export function actionFor(p: Provider, action: string, fields: Partial<Action> = {}): Action {
  return { key_ids: null, provider_id: p.id, expected_revision: p.revision, action, price: null, catalog_provider: '', target: '', secret: '', enabled: true, approved: false, copy_keys: false, strategy: 'round-robin', model: '', multiplier: 0, protocol: '', model_revision: 0, tools: false, streaming: false, ...fields };
}
const count = (v: number) => Number.isSafeInteger(v) && v >= 0;
export function checkedPrice(p: Price): Price {
  const amount = (v: number) => typeof v === 'number' && Number.isFinite(v) && v >= 0 && v <= 1e6;
  if (!p || typeof p.model !== 'string' || typeof p.source !== 'string' || !count(p.updated_at) || !amount(p.input_usd) || !amount(p.output_usd) || (p.cache_read_usd !== null && !amount(p.cache_read_usd)) || (p.cache_write_usd !== null && !amount(p.cache_write_usd))) throw new ProviderProblem('read_error');
  return { model: p.model, input_usd: p.input_usd, output_usd: p.output_usd, cache_read_usd: p.cache_read_usd, cache_write_usd: p.cache_write_usd, source: p.source, updated_at: p.updated_at };
}
function checkedProbe(p: Probe | null): Probe | null {
  if (!p) return null;
  if (![p.revision, p.at, p.http_status, p.elapsed_ms].every(count) || typeof p.key_id !== 'string' || typeof p.model !== 'string' || !['', 'busy', 'authentication', 'rate_limited', 'network', 'timeout', 'invalid_response', 'unexpected_html', 'http_error'].includes(p.failure)) throw new ProviderProblem('read_error');
  return { revision: p.revision, at: p.at, http_status: p.http_status, elapsed_ms: p.elapsed_ms, key_id: p.key_id, model: p.model, failure: p.failure };
}
export function checkedWorkspace(w: Workspace): Workspace {
  if (!w || typeof w.provider_id !== 'string' || !count(w.revision) || typeof w.deleted !== 'boolean' || !Array.isArray(w.keys) || !Array.isArray(w.models) || !Array.isArray(w.usage) || !Array.isArray(w.prices) || !Array.isArray(w.estimates) || (!w.deleted && !['priority', 'round-robin'].includes(w.strategy))) throw new ProviderProblem('read_error');
  return { prices: w.prices.map(checkedPrice), imported_price: w.imported_price == null ? null : checkedPrice(w.imported_price), estimates: w.estimates.map(e => {
    if (typeof e.model !== 'string' || (e.usd !== null && (!Number.isFinite(e.usd) || e.usd < 0))) throw new ProviderProblem('read_error');
    return { model: e.model, usd: e.usd };
  }), provider_id: w.provider_id, revision: w.revision, deleted: w.deleted, strategy: w.strategy, probe: checkedProbe(w.probe), keys: w.keys.map(k => {
    if (!Object.hasOwn(protocolName, k.protocol) || !Array.isArray(k.model_protocols) || k.model_protocols.some(m => typeof m.model !== 'string' || !m.model || !Object.hasOwn(protocolName, m.protocol)) || new Set(k.model_protocols.map(m => m.model)).size !== k.model_protocols.length || typeof k.id !== 'string' || typeof k.enabled !== 'boolean' || !count(k.failures) || !count(k.retry_at) || !['ready', 'disabled', 'cooling', 'invalid', 'half-open'].includes(k.state)) throw new ProviderProblem('read_error');
    return { id: k.id, protocol: k.protocol, model_protocols: k.model_protocols.map(m => ({ model: m.model, protocol: m.protocol })), enabled: k.enabled, state: k.state, failures: k.failures, retry_at: k.retry_at, test: checkedProbe(k.test) };
  }), models: w.models.map(m => {
    if (![m.revision, m.candidate_count, m.candidate_index].every(count) || m.candidate_index >= m.candidate_count || !Array.isArray(m.key_ids) || m.key_ids.length > 64 || m.key_ids.some(id => typeof id !== 'string' || !id) || new Set(m.key_ids).size !== m.key_ids.length || typeof m.id !== 'string' || typeof m.upstream !== 'string' || typeof m.display_name !== 'string' || typeof m.tools !== 'boolean' || typeof m.streaming !== 'boolean' || !Number.isFinite(m.multiplier) || m.multiplier < 0.01 || m.multiplier > 1000 || !validRoutingRule(m.strategy)) throw new ProviderProblem('read_error');
    return { key_ids: [...m.key_ids], candidate_index: m.candidate_index, id: m.id, display_name: m.display_name, upstream: m.upstream, tools: m.tools, streaming: m.streaming, revision: m.revision, candidate_count: m.candidate_count, multiplier: m.multiplier, strategy: m.strategy };
  }), usage: w.usage.map(u => {
    const { provider_id, model, requests, failures, elapsed_ms, known, input_tokens, output_tokens, cache_read, cache_write } = u;
    if (typeof provider_id !== 'string' || typeof model !== 'string' || ![requests, failures, elapsed_ms, known, input_tokens, output_tokens, cache_read, cache_write].every(count) || failures > requests || known > requests) throw new ProviderProblem('read_error');
    return { provider_id, model, requests, failures, elapsed_ms, known, input_tokens, output_tokens, cache_read, cache_write };
  }) };
}
export const healthText: Record<string, string> = { ready: '可调度', disabled: '已停用', cooling: '冷却中', invalid: '认证失效', 'half-open': '等待单次探测' };

export const isValidModelAlias = isValidId;

/** Plan only new mappings; existing aliases and invalid public IDs require manual editing. */
export function catalogImportPlan(ids: string[], w: Workspace) {
  const unique = [...new Set(ids)];
  const mapped = new Set(w.models.map(m => m.upstream));
  const aliases = new Set(w.models.map(m => m.id));
  const pending = unique.filter(id => !mapped.has(id));
  const manual = pending.filter(id => !isValidModelAlias(id) || aliases.has(id));
  return { ready: pending.filter(id => isValidModelAlias(id) && !aliases.has(id)), mapped: unique.length - pending.length, manual };
}
