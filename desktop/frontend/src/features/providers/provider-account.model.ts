import type { AccountSettings, SaveAccountSettings, AccountQuery, AccountAmount, AccountAllowance, AccountObservation } from '../../desktop/contract.generated';
import { ProviderProblem, requestURLError } from './provider.model';
export type { AccountSettings, SaveAccountSettings, AccountQuery, AccountAmount, AccountAllowance, AccountObservation };
export const platformLabels: Record<string, string> = { sub2api: 'sub2api', newapi: 'NewAPI', oneapi: 'OneAPI' };
export interface AccountTransport {
  accountSettings?(id: string, signal: AbortSignal): Promise<AccountSettings>;
  saveAccount?(input: SaveAccountSettings, signal: AbortSignal): Promise<AccountSettings>;
  allowance?(input: AccountQuery, signal: AbortSignal): Promise<AccountObservation>;
}
const safeID = (n: number) => Number.isSafeInteger(n) && n >= 0;
export function checkedAccountSettings(v: AccountSettings, id: string): AccountSettings {
  if (!v || v.provider_id !== id || !safeID(v.revision) || v.revision < 1 || !['', ...Object.keys(platformLabels)].includes(v.platform) ||
    typeof v.site_url !== 'string' || (v.site_url !== '' && !!requestURLError(v.site_url)) || !safeID(v.user_id) || typeof v.has_user_token !== 'boolean') throw new ProviderProblem('read_error');
  return { provider_id: v.provider_id, revision: v.revision, platform: v.platform, site_url: v.site_url, user_id: v.user_id, has_user_token: v.has_user_token };
}
function amount(v: AccountAmount | null): AccountAmount | null {
  if (v === null) return null;
  if (!v || typeof v.value !== 'string' || !/^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(v.value) || v.value.length > 128 || !['USD', 'quota'].includes(v.unit)) throw new ProviderProblem('read_error');
  return { value: v.value, unit: v.unit };
}
function allowance(v: AccountAllowance, scope: string): AccountAllowance {
  if (!v || v.scope !== scope || (v.unlimited !== null && typeof v.unlimited !== 'boolean') || !Array.isArray(v.windows) || v.windows.length > 32) throw new ProviderProblem('read_error');
  return { scope: v.scope, remaining: amount(v.remaining), used: amount(v.used), limit: amount(v.limit), unlimited: v.unlimited,
    windows: v.windows.map(w => {
      if (!w || typeof w.period !== 'string' || w.period.length > 64 || typeof w.reset_at !== 'string' || w.reset_at.length > 128) throw new ProviderProblem('read_error');
      return { period: w.period, remaining: amount(w.remaining), used: amount(w.used), limit: amount(w.limit), reset_at: w.reset_at };
    }) };
}
export function checkedAccountObservation(v: AccountObservation, q: AccountQuery): AccountObservation {
  if (!v || v.provider_id !== q.provider_id || v.revision !== q.expected_revision || v.operation !== q.operation || !['balance', 'key-quota'].includes(v.operation) ||
    !['', ...Object.keys(platformLabels)].includes(v.platform) || typeof v.key_id !== 'string' || (q.key_id && v.key_id !== q.key_id) || v.remote_key_id !== q.remote_key_id ||
    typeof v.observed_at !== 'string' || !Number.isFinite(Date.parse(v.observed_at)) || !Number.isInteger(v.http_status) || v.http_status < 0 || v.http_status > 599 ||
    typeof v.failure !== 'string' || v.failure.length > 64 || (v.failure ? v.allowance !== null : v.allowance === null)) throw new ProviderProblem('read_error');
  return { provider_id: v.provider_id, revision: v.revision, platform: v.platform, operation: v.operation, key_id: v.key_id, remote_key_id: v.remote_key_id,
    observed_at: v.observed_at, http_status: v.http_status, failure: v.failure, allowance: v.allowance === null ? null : allowance(v.allowance, q.operation === 'balance' ? 'account' : 'key') };
}
export function amountText(v: AccountAmount | null): string {
  return v === null ? '未知' : `${v.value} ${v.unit === 'USD' ? 'USD' : 'quota（原始额度）'}`;
}
export const accountFailureText: Record<string, string> = {
  unknown: '平台没有返回明确的额度或周期窗口，当前额度未知。',
  platform_required: '请先选择并保存平台类型。',
  auth_required: '当前查询需要额外授权。请配置平台用户令牌，或检查所选推理 Key；Key 限额不能代表账户余额。',
  remote_key_required: 'OneAPI 查询 Key 额度需要填写该站后台的令牌 ID。',
  authentication: '平台拒绝授权。请核对令牌是否有效，以及 NewAPI 的用户 ID。',
  unsupported: '该站未提供此功能或使用了不兼容的接口。',
  invalid_response: '平台响应缺少可识别的额度字段，无法确认数值。',
  rate_limited: '平台限制了请求频率，请稍后刷新。',
  busy: '供应商正在处理其他请求，请稍后刷新。',
  network: '无法连接平台，请检查地址、网络及代理。',
  timeout: '平台查询超时，请稍后刷新。',
  http_error: '平台接口请求失败，请检查站点是否支持此功能。',
  too_large: '平台响应超过安全读取范围。',
  invalid_request: '查询参数无效，请重新读取配置。',
};
