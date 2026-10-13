import type { AccountSettings, SaveAccountSettings, AccountQuery, AccountAmount, AccountAllowance, AccountObservation, AccountRate, AccountRateObservation } from '../../desktop/contract.generated';
import { ProviderProblem, requestURLError } from './provider.model';
export type { AccountSettings, SaveAccountSettings, AccountQuery, AccountAmount, AccountAllowance, AccountObservation, AccountRate, AccountRateObservation };
export const platformLabels: Record<string, string> = { auto: '自动探测', sub2api: 'sub2api', newapi: 'NewAPI', oneapi: 'OneAPI' };
export interface AccountTransport {
  accountSettings?(id: string, signal: AbortSignal): Promise<AccountSettings>;
  saveAccount?(input: SaveAccountSettings, signal: AbortSignal): Promise<AccountSettings>;
  allowance?(input: AccountQuery, signal: AbortSignal): Promise<AccountObservation>;
  rate?(input: AccountQuery, signal: AbortSignal): Promise<AccountRateObservation>;
}
const safeID = (n: number) => Number.isSafeInteger(n) && n >= 0;
export function checkedAccountSettings(v: AccountSettings, id: string): AccountSettings {
  if (!v || v.provider_id !== id || !safeID(v.revision) || v.revision < 1 || !['', ...Object.keys(platformLabels)].includes(v.platform) ||
    typeof v.site_url !== 'string' || (v.site_url !== '' && !!requestURLError(v.site_url)) || !safeID(v.user_id) || typeof v.has_user_token !== 'boolean') throw new ProviderProblem('read_error');
  return { provider_id: v.provider_id, revision: v.revision, platform: v.platform, site_url: v.site_url, user_id: v.user_id, has_user_token: v.has_user_token };
}
function amount(v: AccountAmount | null): AccountAmount | null {
  if (v === null) return null;
  if (!v || typeof v.value !== 'string' || !/^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(v.value) || v.value.length > 512 || !['USD', 'CNY', 'quota', 'site_unit'].includes(v.unit)) throw new ProviderProblem('read_error');
  return { value: v.value, unit: v.unit };
}
function allowance(v: AccountAllowance, scopes: string[]): AccountAllowance {
  if (!v || !scopes.includes(v.scope) || (v.unlimited !== null && typeof v.unlimited !== 'boolean') || !Array.isArray(v.windows) || v.windows.length > 32) throw new ProviderProblem('read_error');
  return { scope: v.scope, remaining: amount(v.remaining), used: amount(v.used), limit: amount(v.limit), unlimited: v.unlimited,
    windows: v.windows.map(w => {
      if (!w || typeof w.period !== 'string' || w.period.length > 64 || typeof w.reset_at !== 'string' || w.reset_at.length > 128) throw new ProviderProblem('read_error');
      return { period: w.period, remaining: amount(w.remaining), used: amount(w.used), limit: amount(w.limit), reset_at: w.reset_at };
    }) };
}
export function checkedAccountObservation(v: AccountObservation, q: AccountQuery): AccountObservation {
  if (!v || v.provider_id !== q.provider_id || v.revision !== q.expected_revision || v.operation !== q.operation || !['balance', 'balance-summary', 'key-quota'].includes(v.operation) ||
    !['', ...Object.keys(platformLabels)].includes(v.platform) || typeof v.key_id !== 'string' || (q.key_id && v.key_id !== q.key_id) || v.remote_key_id !== q.remote_key_id ||
    typeof v.observed_at !== 'string' || !Number.isFinite(Date.parse(v.observed_at)) || !Number.isInteger(v.http_status) || v.http_status < 0 || v.http_status > 599 ||
    (!v.failure && ['', 'auto'].includes(v.platform)) || typeof v.failure !== 'string' || v.failure.length > 64 || (v.failure ? v.allowance !== null : v.allowance === null)) throw new ProviderProblem('read_error');
  return { provider_id: v.provider_id, revision: v.revision, platform: v.platform, operation: v.operation, key_id: v.key_id, remote_key_id: v.remote_key_id,
    observed_at: v.observed_at, http_status: v.http_status, failure: v.failure, allowance: v.allowance === null ? null : allowance(v.allowance, q.operation === 'key-quota' ? ['key'] : v.platform === 'newapi' ? ['account', 'key', 'billing'] : q.operation === 'balance-summary' ? ['account', 'key'] : ['account']) };
}
export function amountText(v: AccountAmount | null): string {
  const units: Record<string, string> = { USD: 'USD', CNY: 'CNY', quota: 'quota（原始额度）', site_unit: '站点计费单位（币种未确认）' };
  return v === null ? '未知' : `${v.value} ${units[v.unit]}`;
}
export function allowanceScopeText(scope: string): string {
  return ({ account: '账户余额', key: 'Key 额度', billing: '站点余额' } as Record<string, string>)[scope] ?? '未知范围';
}
export const billingScopeHint = '站点 billing 接口返回的余额；查询范围由站点设置决定，可能是账户或所选 Key。查询账户钱包可配置用户授权。';
export const accountFailureText: Record<string, string> = {
  unknown: '平台没有返回明确的额度或周期窗口，当前额度未知。',
  platform_unknown: '无法自动识别平台，可在账户页指定平台类型。',
  permission_denied: '该 Key 没有查询权限。',
  upstream_rejected: '平台拒绝了查询请求。',
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

export function checkedAccountRate(v: AccountRateObservation, q: AccountQuery): AccountRateObservation {
  if (!v || q.operation !== 'effective-rate' || v.operation !== q.operation || v.provider_id !== q.provider_id || v.revision !== q.expected_revision ||
    !['', ...Object.keys(platformLabels)].includes(v.platform) || typeof v.key_id !== 'string' || !v.key_id || (q.key_id && v.key_id !== q.key_id) || v.remote_key_id !== q.remote_key_id ||
    typeof v.observed_at !== 'string' || !Number.isFinite(Date.parse(v.observed_at)) || !Number.isInteger(v.http_status) || v.http_status < 0 || v.http_status > 599 ||
    typeof v.failure !== 'string' || v.failure.length > 64 || (!v.failure && ['', 'auto'].includes(v.platform))) throw new ProviderProblem('read_error');
  const r = v.rate;
  if (r !== null && (!r || typeof r.group !== 'string' || (!r.group && r.basis !== 'key_billing') || r.group.length > 128 || typeof r.group_name !== 'string' || r.group_name.length > 256 ||
    !['personal_override', 'authenticated_group', 'key_billing'].includes(r.basis) ||
    (r.basis === 'key_billing' && (v.platform !== 'sub2api' || v.remote_key_id !== 0)) ||
    (r.multiplier !== null && (typeof r.multiplier !== 'string' || r.multiplier.length > 128 || !/^(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(r.multiplier) || !Number.isFinite(Number(r.multiplier)) || Number(r.multiplier) < 0)))) throw new ProviderProblem('read_error');
  if (v.failure ? r?.multiplier != null : r?.multiplier == null) throw new ProviderProblem('read_error');
  return { provider_id: v.provider_id, revision: v.revision, platform: v.platform, operation: v.operation, key_id: v.key_id, remote_key_id: v.remote_key_id,
    observed_at: v.observed_at, http_status: v.http_status, failure: v.failure,
    rate: r === null ? null : { group: r.group, group_name: r.group_name, multiplier: r.multiplier, basis: r.basis } };
}
export const rateBasisText: Record<string, string> = { personal_override: '个人倍率覆盖', authenticated_group: '已认证分组倍率', key_billing: 'Key 当前计费倍率（含个人覆盖及高峰规则）' };
export const rateFailureText: Record<string, string> = {
  ...accountFailureText,
  auth_required: '当前查询需要用户授权。sub2api 的所选 Key 优先直接读取倍率；旧站缺少此接口、指定后台 Key ID，或 NewAPI 查询时需配置用户令牌。',
  remote_key_required: '请填写该站后台的 Key ID，或留空以核对当前保存的 Key。',
  key_not_found: '平台账户中未找到所选 Key；请核对用户授权、Key 或后台 Key ID。',
  unknown: '分组倍率未知。自动分组可能按请求选择分组，无法显示一个固定倍率。',
  unsupported: '该平台未提供可读取的分组倍率；原版 OneAPI 不支持此查询。',
  invalid_response: '平台没有返回有效的分组倍率，无法确认数值。',
};
