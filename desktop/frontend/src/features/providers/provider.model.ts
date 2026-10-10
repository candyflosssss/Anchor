import type { Provider as DesktopProvider, SaveProvider } from "../../desktop/contract.generated";
export type { SaveProvider };
export type Provider = DesktopProvider & { protocol: "responses" };
export type SecretMode = 'keep' | 'replace' | 'clear';
export interface ProviderDraft {
  id: string;
  base_url: string;
  enabled: boolean;
  timeout: string;
  concurrent: string;
  secretMode: SecretMode;
  secret: string;
  confirmClear: boolean;
}
export type FieldErrors = Partial<Record<'id' | 'base_url' | 'timeout' | 'concurrent' | 'secret', string>>;
export type ProblemCode = 'offline' | 'unauthorized' | 'read_error' | 'conflict' | 'unknown' | 'invalid';
export class ProviderProblem extends Error {
  constructor(readonly code: ProblemCode) { super(code); }
}
export const problemText: Record<ProblemCode, string> = {
  offline: '实例不可用。请检查核心是否运行；不会自动回退到其他实例。',
  unauthorized: '管理授权不可用。请在桌面连接层重新确认授权，页面不接收管理令牌。',
  read_error: '未能读取供应商配置。可以重试读取，不会改变已保存配置。',
  conflict: '配置已被其他操作修改。未覆盖新版本；请重新读取后再编辑。',
  unknown: '写入结果未知，可能已经保存。请先重新读取并核对配置与 revision，不要直接重试。',
  invalid: '配置未通过校验。请核对字段后重新提交。',
};
export interface ProviderTransport {
  list(signal: AbortSignal): Promise<Provider[]>;
  save(input: SaveProvider, signal: AbortSignal): Promise<Provider>;
}
export function newDraft(provider?: Provider): ProviderDraft {
  return {
    id: provider?.id ?? '', base_url: provider?.base_url ?? '', enabled: provider?.enabled ?? true,
    timeout: String(provider?.timeout_seconds ?? 120), concurrent: String(provider?.max_concurrent ?? 4),
    secretMode: provider ? 'keep' : 'replace', secret: '', confirmClear: false,
  };
}
export function validateDraft(draft: ProviderDraft, creating: boolean): FieldErrors {
  const errors: FieldErrors = {};
  if (!/^[\p{L}\p{Nd}][\p{L}\p{Nd}._-]{0,127}$/u.test(draft.id)) errors.id = '1–128 个中文／字母／数字或 ._-，以字母或数字开头。';
  try {
    const raw = draft.base_url.trim();
    const url = new URL(raw);
    const authority = raw.match(/^https?:\/\/([^/]+)/)?.[1] ?? '';
    const numericLoopback = /^127(?:\.\d{1,3}){3}(?::\d+)?$/.test(authority) || /^\[::1\](?::\d+)?$/.test(authority);
    if (!url.hostname || url.username || url.password || raw.includes('?') || raw.includes('#') || /\s/.test(raw) || raw.includes('\\')) throw new Error();
    if (url.protocol !== 'https:' && !(url.protocol === 'http:' && numericLoopback)) throw new Error();
    if (/\/responses\/?$/.test(url.pathname)) errors.base_url = '填写 API 根地址（可含 /v1），不包含 /responses。';
  } catch { errors.base_url = '请输入 HTTPS 根地址，不含账号、查询或片段；HTTP 仅限数值回环测试地址。'; }
  if (!/^\d+$/.test(draft.timeout) || +draft.timeout < 1 || +draft.timeout > 3600) errors.timeout = '请输入 1–3600 的整数秒数。';
  if (!/^\d+$/.test(draft.concurrent) || +draft.concurrent < 1 || +draft.concurrent > 64) errors.concurrent = '请输入 1–64 的整数。';
  const secret = draft.secret.trim();
  if (draft.secretMode === 'replace' && ((creating ? false : !secret) || /[\r\n]/.test(secret) || new TextEncoder().encode(secret).length > 8192)) errors.secret = '替换密钥须非空、无内部换行，且不超过 8192 字节。';
  if (draft.secretMode === 'clear' && !draft.confirmClear) errors.secret = '请确认清除密钥的影响。';
  return errors;
}
export function toSave(draft: ProviderDraft, source?: Provider): SaveProvider {
  const expected_revision = source?.revision ?? 0;
  if (!Number.isSafeInteger(expected_revision) || expected_revision < 0) throw new ProviderProblem('invalid');
  return {
    provider: { id: draft.id, base_url: draft.base_url.trim(), protocol: 'responses', enabled: draft.enabled,
      timeout_seconds: +draft.timeout, max_concurrent: +draft.concurrent,
      revision: expected_revision, has_secret: source?.has_secret ?? false },
    expected_revision,
    secret: draft.secretMode === 'keep' ? null : draft.secretMode === 'clear' ? '' : draft.secret.trim(),
  };
}
export function isDirty(draft: ProviderDraft, source?: Provider): boolean {
  return JSON.stringify(draft) !== JSON.stringify(newDraft(source));
}
