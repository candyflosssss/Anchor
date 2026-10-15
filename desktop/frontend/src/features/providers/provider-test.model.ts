import { ProviderProblem } from './provider.model';
import type { ProbeProgress, ProbeRequest, ProviderModels, Workspace } from '../../desktop/contract.generated';

/** A configured mapping must never fall back outside its Key binding. */
export function keyForMappingTest(workspace: Workspace, keyIds: string[]): string {
  if (!workspace.keys.length) return !keyIds.length || keyIds.includes('anonymous') ? 'anonymous' : '';
  if (keyIds.length) return keyIds.find(id => workspace.keys.some(key => key.id === id && key.enabled)) ?? '';
  return workspace.keys.find(key => key.enabled)?.id ?? '';
}

export function modelsForKey(catalog: ProviderModels, key: string): string[] {
  return catalog.entries.filter(entry => entry.key_ids.includes(key)).map(entry => entry.id);
}

export function checkedProgress(value: ProbeProgress, input: ProbeRequest): ProbeProgress {
  if (!value || value.provider_id !== input.provider_id || value.revision !== input.expected_revision ||
    typeof value.run_id !== 'string' || !value.run_id || (input.run_id && value.run_id !== input.run_id) ||
    typeof value.key_id !== 'string' || typeof value.model !== 'string' ||
    (input.action === 'start' && (value.key_id !== input.key_id || value.model !== input.model)) ||
    !['running', 'completed', 'cancelled'].includes(value.status) || !['connecting', 'receiving', 'complete'].includes(value.phase) ||
    typeof value.text !== 'string' || new TextEncoder().encode(value.text).length > 16384 ||
    !Number.isSafeInteger(value.elapsed_ms) || value.elapsed_ms < 0 ||
    (value.status === 'running' ? value.result !== null : !value.result)) throw new ProviderProblem('read_error');
  const result = value.result;
  if (result && (result.revision !== value.revision || result.key_id !== value.key_id || result.model !== value.model ||
    ![result.at, result.elapsed_ms, result.http_status].every(n => Number.isSafeInteger(n) && n >= 0) ||
    !['', 'busy', 'authentication', 'rate_limited', 'network', 'timeout', 'invalid_response', 'unexpected_html', 'http_error'].includes(result.failure))) throw new ProviderProblem('read_error');
  return { provider_id: value.provider_id, revision: value.revision, run_id: value.run_id, key_id: value.key_id,
    model: value.model, status: value.status, phase: value.phase, text: value.text, elapsed_ms: value.elapsed_ms,
    result: result ? { revision: result.revision, key_id: result.key_id, model: result.model, at: result.at,
      http_status: result.http_status, elapsed_ms: result.elapsed_ms, failure: result.failure } : null };
}

export function probeFailureText(code: string): string {
  if (code === 'unexpected_html') return '上游返回了 HTML 网页，而不是模型响应。请检查请求地址是否为 API 地址（通常以 /v1 结尾），不要填写网站主页。';
  return code;
}
