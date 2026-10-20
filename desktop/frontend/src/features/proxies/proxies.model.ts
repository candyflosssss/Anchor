import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { ProxyAction, ProxyBinding, ProxyWorkspace } from '../../desktop/contract.generated';
export type { ProxyAction, ProxyBinding, ProxyWorkspace } from '../../desktop/contract.generated';
export function bindingValue(workspace: ProxyWorkspace, provider: string, key: string) {
  const binding = workspace.bindings.find(b => b.provider_id === provider && b.key_id === key);
  return binding?.mode === 'proxy' ? `proxy:${binding.proxy_id}` : binding?.mode ?? (key ? 'inherit' : 'direct');
}
export function bindingFromValue(provider: string, key: string, value: string): ProxyBinding {
  return { provider_id: provider, key_id: key, mode: value.startsWith('proxy:') ? 'proxy' : value, proxy_id: value.startsWith('proxy:') ? value.slice(6) : '' };
}
export function checkedWorkspace(data: ProxyWorkspace): ProxyWorkspace {
  if (!data || !Number.isSafeInteger(data.revision) || data.revision < 1 || !Array.isArray(data.profiles) || !Array.isArray(data.bindings) || !Array.isArray(data.targets) ||
    data.profiles.some(p => !p.id || typeof p.name !== 'string' || !['http', 'socks5', 'mihomo'].includes(p.kind) || !Number.isSafeInteger(p.revision)) ||
    data.bindings.some(b => typeof b.provider_id !== 'string' || typeof b.key_id !== 'string' || typeof b.proxy_id !== 'string' || !['direct', 'system', 'inherit', 'proxy'].includes(b.mode)) ||
    data.targets.some(t => typeof t.provider_id !== 'string' || typeof t.key_id !== 'string') || !data.engine || typeof data.engine.available !== 'boolean' || !Number.isSafeInteger(data.engine.running)) throw new DesktopProblem('read_error');
  // Explicitly pick public fields; even a malformed remote response cannot put secrets in UI state.
  return { revision: data.revision, profiles: data.profiles.map(p => ({ id: p.id, name: p.name, kind: p.kind, revision: p.revision })),
    bindings: data.bindings.map(b => ({ provider_id: b.provider_id, key_id: b.key_id, mode: b.mode, proxy_id: b.proxy_id })),
    targets: data.targets.map(t => ({ provider_id: t.provider_id, key_id: t.key_id })),
    engine: { available: data.engine.available, version: data.engine.version, platform: data.engine.platform, running: data.engine.running } };
}
export const proxyModel = {
  async read() { return checkedWorkspace(await call(() => bridge().ProxyWorkspace())); },
  async action(input: ProxyAction) { return checkedWorkspace(await call(() => bridge().ManageProxy(input), true)); },
};
export function proxyAction(action: string, revision: number): ProxyAction {
  return { action, expected_revision: revision, approved: true, profile: { id: '', name: '', kind: '', revision: 0 }, secret: null, node_name: '', binding: { provider_id: '', key_id: '', mode: '', proxy_id: '' } };
}
export function proxyError(error: unknown) {
  if (error instanceof DesktopProblem) {
    switch (error.code) {
      case 'unsupported': return '当前核心不支持代理设置，请先升级核心。';
      case 'conflict': return '配置已被其他操作修改，请重新读取后再保存。';
      case 'referenced': return '此代理仍被供应商或 Key 引用，请先改为不使用应用代理、系统代理、继承或其他代理。';
      case 'invalid': return '配置无效：检查 ID、代理 URL、节点 YAML 和节点名称。';
      case 'unknown': return '写入结果未知。请重新读取确认，不要直接重复提交。';
      case 'unauthorized': return '没有管理权限，请检查当前连接。';
    }
  }
  return '无法读取或保存代理配置，请检查核心连接后重新读取。';
}
