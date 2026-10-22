import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { Access, APIServiceWorkspace, DesktopBridge, IssueAPIKey, SaveAPIServiceSettings } from '../../desktop/contract.generated';

export function checkedAccess(value: Access): Access {
  if (!value || typeof value.id !== 'string' || !value.id || typeof value.label !== 'string' || value.label.length > 128 || value.scope !== 'gateway' || typeof value.revoked !== 'boolean' || !Number.isSafeInteger(value.expires_at) || value.expires_at < 0) throw new DesktopProblem('read_error');
  return { id: value.id, label: value.label, scope: value.scope, revoked: value.revoked, expires_at: value.expires_at };
}
export function checkedAPIService(value: APIServiceWorkspace): APIServiceWorkspace {
  const s = value?.settings;
  if (!s || typeof s.enabled !== 'boolean' || typeof s.allow_lan !== 'boolean' || !Number.isInteger(s.port) || s.port < 1 || s.port > 65535 || !Number.isSafeInteger(s.revision) || s.revision < 1 || typeof value.active_address !== 'string' || typeof value.restart_required !== 'boolean' || !Array.isArray(value.base_urls) || value.base_urls.some(url => typeof url !== 'string' || !/^http:\/\/(127\.0\.0\.1|10\.[\d.]+|192\.168\.[\d.]+|172\.(1[6-9]|2\d|3[01])\.[\d.]+):\d+\/v1$/.test(url)) || !Array.isArray(value.keys)) throw new DesktopProblem('read_error');
  return { settings: { enabled: s.enabled, port: s.port, allow_lan: s.allow_lan, revision: s.revision }, active_address: value.active_address, restart_required: value.restart_required, base_urls: [...value.base_urls], keys: value.keys.map(checkedAccess) };
}
export function apiInteger(text: string, max: number, min = 0): number | null {
  const value = Number(text); return /^\d+$/.test(text) && Number.isSafeInteger(value) && value >= min && value <= max ? value : null;
}
export function createAPIService(get: () => Pick<DesktopBridge, 'APIService' | 'SaveAPIService' | 'IssueAPIKey' | 'RevokeAPIKey'> = bridge) {
  return {
    read: async () => checkedAPIService(await call(() => get().APIService())),
    async save(input: SaveAPIServiceSettings) {
      const value = await call(() => get().SaveAPIService(input), true);
      try { const out = checkedAPIService(value); if (out.settings.revision !== input.expected_revision + 1) throw new Error(); return out; } catch { throw new DesktopProblem('unknown'); }
    },
    async issue(input: IssueAPIKey) {
      const out = await call(() => get().IssueAPIKey(input), true);
      try { const access = checkedAccess(out.access); if (typeof out.token !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(out.token)) throw new Error(); return { access, token: out.token }; } catch { throw new DesktopProblem('unknown'); }
    },
    async revoke(id: string) {
      const out = await call(() => get().RevokeAPIKey({ id }), true);
      try { return checkedAPIService(out); } catch { throw new DesktopProblem('unknown'); }
    },
  };
}
export const apiService = createAPIService();
export function apiError(error: unknown): string {
  const code = error instanceof DesktopProblem ? error.code : 'read_error';
  return code === 'conflict' ? '配置已变化，请重新读取后保存。' : code === 'unknown' ? '操作结果未知，请重新读取；签发的 Key 无法再次查看，可撤销后重新签发。' : code === 'unsupported' ? '当前核心不支持，请升级核心。' : code === 'invalid' ? '参数无效，请检查端口、名称和有效期。' : code === 'unauthorized' ? '没有管理权限。' : '读取或连接失败，请检查核心状态。';
}
