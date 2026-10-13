import type { AccountTransport } from './provider-account.model';
import type { Provider as DesktopProvider, SaveProvider, DiscoverProviderModels, ProviderModels } from "../../desktop/contract.generated";
export type { SaveProvider, DiscoverProviderModels, ProviderModels };
export type Provider = DesktopProvider;
export const MAX_PROVIDER_URLS = 16;
export const protocolName: Record<string, string> = { responses: "Responses", "chat-completions": "Chat Completions", anthropic: "Anthropic Messages" };
export type SecretMode = 'keep' | 'replace' | 'clear';
export interface ProviderDraft {
  id: string;
  homepage_url: string;
  base_urls: string[];
  enabled: boolean;
  timeout: string;
  concurrent: string;
  secretMode: SecretMode;
  secret: string;
  confirmClear: boolean;
}
export type FieldErrors = Partial<Record<'id' | 'homepage_url' | 'base_urls' | 'timeout' | 'concurrent' | 'secret' | `base_url_${number}`, string>>;
export type ProblemCode = 'offline' | 'unauthorized' | 'read_error' | 'conflict' | 'unknown' | 'invalid' | 'referenced';
export class ProviderProblem extends Error {
  constructor(readonly code: ProblemCode) { super(code); }
}
export const problemText: Record<ProblemCode, string> = {
  offline: '实例不可用。请检查核心是否运行；不会自动回退到其他实例。',
  unauthorized: '管理授权不可用。请在桌面连接层重新确认授权，页面不接收管理令牌。',
  read_error: '未能读取供应商配置。可以重试读取，不会改变已保存配置。',
  conflict: '配置已被其他操作修改。未覆盖新版本；请重新读取后再编辑。',
  unknown: '写入结果未知，可能已经保存。请先重新读取并核对配置与 revision，不要直接重试。',
  referenced: '仍有模型或 Binding 引用此项目。请先移除引用，再执行删除。',
  invalid: '配置未通过校验。请核对字段后重新提交。',
};
export interface ProviderTransport extends AccountTransport {
  workspace?(id: string, signal: AbortSignal): Promise<import("../../desktop/contract.generated").Workspace>;
  action?(input: import("../../desktop/contract.generated").Action, signal: AbortSignal): Promise<import("../../desktop/contract.generated").Workspace>;
  list(signal: AbortSignal): Promise<Provider[]>;
  discover(input: DiscoverProviderModels, signal: AbortSignal): Promise<ProviderModels>;
  save(input: SaveProvider, signal: AbortSignal): Promise<Provider>;
}
export function newDraft(provider?: Provider): ProviderDraft {
  return {
    id: provider?.id ?? '', homepage_url: provider?.homepage_url ?? '', base_urls: provider?.base_urls.length ? [...provider.base_urls] : [''], enabled: provider?.enabled ?? true,
    timeout: String(provider?.timeout_seconds ?? 120), concurrent: String(provider?.max_concurrent ?? 4),
    secretMode: 'keep', secret: '', confirmClear: false,
  };
}
export function validateDraft(draft: ProviderDraft): FieldErrors {
  const errors: FieldErrors = {};
  if (!/^[\p{L}\p{Nd}][\p{L}\p{Nd}._-]{0,127}$/u.test(draft.id)) errors.id = '1–128 个中文／字母／数字或 ._-，以字母或数字开头。';
  const urls = draft.base_urls.map(u => u.trim());
  if (!urls.length || urls.length > MAX_PROVIDER_URLS) errors.base_urls = `请填写 1–${MAX_PROVIDER_URLS} 个请求地址。`;
  const seen = new Set<string>();
  urls.forEach((raw, i) => {
    const message = requestURLError(raw);
    const key = raw.replace(/\/+$/, '');
    if (message) errors[`base_url_${i}`] = message;
    else if (seen.has(key)) errors[`base_url_${i}`] = '请求地址重复。';
    seen.add(key);
  });
  const homepage = draft.homepage_url.trim();
  if (homepage) {
    try {
      const url = new URL(homepage);
      if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || /\s/.test(homepage) || homepage.length > 2048) throw new Error();
    } catch { errors.homepage_url = '请输入 http(s) 主页地址，不含账号信息；可留空。'; }
  }
  if (!/^\d+$/.test(draft.timeout) || +draft.timeout < 1 || +draft.timeout > 3600) errors.timeout = '请输入 1–3600 的整数秒数。';
  if (!/^\d+$/.test(draft.concurrent) || +draft.concurrent < 1 || +draft.concurrent > 64) errors.concurrent = '请输入 1–64 的整数。';
  const secret = draft.secret.trim();
  if (draft.secretMode === 'replace' && (!secret || /[\r\n]/.test(secret) || new TextEncoder().encode(secret).length > 8192)) errors.secret = '替换密钥须非空、无内部换行，且不超过 8192 字节。';
  if (draft.secretMode === 'clear' && !draft.confirmClear) errors.secret = '请确认清除密钥的影响。';
  return errors;
}
export function requestURLError(raw: string): string | undefined {
  try {
    const url = new URL(raw);
    const authority = raw.match(/^https?:\/\/([^/]+)/)?.[1] ?? '';
    const numericLoopback = /^127(?:\.\d{1,3}){3}(?::\d+)?$/.test(authority) || /^\[::1\](?::\d+)?$/.test(authority);
    if (!url.hostname || url.username || url.password || raw.includes('?') || raw.includes('#') || /\s/.test(raw) || raw.includes('\\')) throw new Error();
    if (url.protocol !== 'https:' && !(url.protocol === 'http:' && numericLoopback)) throw new Error();
    if (/\/responses\/?$/.test(url.pathname)) return '填写 API 根地址（可含 /v1），不包含 /responses。';
  } catch { return '请输入 HTTPS 根地址，不含账号、查询或片段；HTTP 仅限数值回环测试地址。'; }
  return undefined;
}
export function toSave(draft: ProviderDraft, source?: Provider): SaveProvider {
  const expected_revision = source?.revision ?? 0;
  if (!Number.isSafeInteger(expected_revision) || expected_revision < 0) throw new ProviderProblem('invalid');
  return {
    provider: { id: draft.id, homepage_url: draft.homepage_url.trim(), base_urls: draft.base_urls.map(u => u.trim()), enabled: draft.enabled,
      timeout_seconds: +draft.timeout, max_concurrent: +draft.concurrent,
      revision: expected_revision, has_secret: source?.has_secret ?? false },
    expected_revision,
    secret: !source || draft.secretMode === 'keep' ? null : draft.secretMode === 'clear' ? '' : draft.secret.trim(),
  };
}
export function isDirty(draft: ProviderDraft, source?: Provider): boolean {
  return JSON.stringify(draft) !== JSON.stringify(newDraft(source));
}

export const catalogFailureText: Record<string, string> = {
  authentication: '上游拒绝目录授权（401 / 403）。请检查已存密钥与目录权限。',
  no_enabled_keys: '没有启用的 Key，已跳过目录和分组倍率查询。',
  unsupported: '上游未提供此模型目录接口（404 / 405）；不代表模型推理不可用。',
  rate_limited: '上游限制目录请求频率。请稍后手动重试。',
  http_error: '上游目录返回非成功 HTTP 状态；未展示其错误正文。',
  network: '目录请求未完成。请检查上游地址和网络；未自动重试。',
  timeout: '目录请求超时（最多 8 秒，受供应商超时限制）。',
  busy: '当前全局或供应商并发已满，未发送目录请求。请稍后重试。',
  too_large: '目录超过大小、条数或分页上限，未返回部分列表。',
  invalid_response: '上游目录格式或分页信息不符合要求，未返回部分列表。',
};
export const catalogRateFailureText: Record<string, string> = {
  platform_required: '尚未配置账户平台', platform_unknown: '无法识别账户平台',
  auth_required: '需要账户授权', authentication: '账户授权被拒绝', permission_denied: '无分组读取权限',
  key_not_found: '账户内未找到此 Key', remote_key_required: '缺少远端 Key 标识',
  unsupported: '此平台不支持倍率查询', unknown: '上游未提供确定倍率',
  timeout: '抓取超时', network: '网络请求失败', rate_limited: '上游限流',
  too_large: '抓取超过安全上限', invalid_response: '上游数据无效', invalid_request: '账户配置无效',
  http_error: '上游请求失败', upstream_rejected: '上游拒绝请求',
};
export function checkedCatalog(value: ProviderModels, input: DiscoverProviderModels): ProviderModels {
  const invalid = () => { throw new ProviderProblem('read_error'); };
  const status = (n: number) => Number.isInteger(n) && (n === 0 || (n >= 100 && n <= 599));
  const safeText = (s: string, limit = 512) => typeof s === 'string' && new TextEncoder().encode(s).length <= limit && !/[\p{Cc}\p{Cf}]/u.test(s);
  const failure = (s: string) => s === '' || (typeof s === 'string' && Object.hasOwn(catalogFailureText, s));
  if (!value || value.provider_id !== input.provider_id || value.revision !== input.expected_revision || typeof value.key_id !== 'string' || (input.key_id !== '' && value.key_id !== input.key_id) ||
    !Number.isSafeInteger(value.revision) || value.revision < 1 || !status(value.http_status) ||
    !Number.isSafeInteger(value.elapsed_ms) || value.elapsed_ms < 0 || !failure(value.failure) ||
    (value.failure === '' && value.http_status !== 200) || typeof value.partial !== 'boolean' ||
    !Array.isArray(value.models) || value.models.length > 1000 || (value.failure !== '' && (value.models.length !== 0 || value.partial)) ||
    value.models.some(id => !safeText(id, 256) || !id || id.trim() !== id) || new Set(value.models).size !== value.models.length ||
    !Array.isArray(value.entries) || value.entries.length !== value.models.length || !Array.isArray(value.keys) || value.keys.length > 64) return invalid();
  const keyIDs = new Set<string>();
  for (const k of value.keys) {
    if (!k || !safeText(k.id) || !k.id || keyIDs.has(k.id) || typeof k.enabled !== 'boolean' || !Object.hasOwn(protocolName, k.protocol) || !status(k.http_status) || !failure(k.failure) ||
      (!k.failure && k.http_status !== 200) || !safeText(k.group) || !safeText(k.group_name) ||
      !['', 'authenticated_group', 'personal_override'].includes(k.rate_basis) ||
      (k.rate_failure !== '' && !Object.hasOwn(catalogRateFailureText, k.rate_failure)) ||
      (k.multiplier !== null && (typeof k.multiplier !== 'string' || !/^(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(k.multiplier) || !Number.isFinite(Number(k.multiplier)) || Number(k.multiplier) < 0 || !k.group || !k.rate_basis || k.rate_failure !== ''))) return invalid();
    keyIDs.add(k.id);
  }
  if (!value.failure && (!value.keys.some(k => !k.failure) || value.partial !== value.keys.some(k => !!k.failure))) return invalid();
  const entryIDs = new Set<string>();
  for (const e of value.entries) {
    if (!e || !value.models.includes(e.id) || entryIDs.has(e.id) || !Array.isArray(e.key_ids) || !e.key_ids.length || new Set(e.key_ids).size !== e.key_ids.length ||
      e.key_ids.some(id => !keyIDs.has(id) || value.keys.find(k => k.id === id)?.failure !== '')) return invalid();
    entryIDs.add(e.id);
  }
  return { ...value, models: [...value.models], entries: value.entries.map(e => ({ ...e, key_ids: [...e.key_ids] })), keys: value.keys.map(k => ({ ...k })) };
}
export function catalogProblemText(code: ProblemCode): string {
  if (code === 'conflict') return '配置版本已变化。请重新读取配置后再查询目录；没有覆盖配置。';
  if (code === 'unauthorized' || code === 'offline') return problemText[code];
  return '未能读取模型目录。配置未改变；可以重新读取配置后手动重试。';
}
