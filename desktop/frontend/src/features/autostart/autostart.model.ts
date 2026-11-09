import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { AutostartState, DesktopBridge, SaveAutostart } from '../../desktop/contract.generated';

export function checkedAutostart(value: unknown): AutostartState {
  const v = value as AutostartState | null;
  if (!v || ['supported', 'enabled', 'matches_current', 'disabled_by_system', 'can_enable'].some(k => typeof v[k as keyof AutostartState] !== 'boolean') ||
    typeof v.revision !== 'string' || (v.supported ? !/^[a-f0-9]{64}$/.test(v.revision) : v.revision !== '') ||
    (!v.supported && (v.enabled || v.matches_current || v.can_enable || v.disabled_by_system)) || (v.matches_current && !v.enabled)) throw new DesktopProblem('read_error');
  return { supported: v.supported, enabled: v.enabled, matches_current: v.matches_current, disabled_by_system: v.disabled_by_system, can_enable: v.can_enable, revision: v.revision };
}
export function createAutostartService(getBridge: () => Pick<DesktopBridge, 'Autostart' | 'SaveAutostart'> = bridge) {
  return {
    async read() {
      const api = getBridge();
      if (!api.Autostart) throw new DesktopProblem('unsupported');
      return checkedAutostart(await call(() => api.Autostart()));
    },
    async save(input: SaveAutostart) {
      if (typeof input.enabled !== 'boolean' || input.approved !== true || !/^[a-f0-9]{64}$/.test(input.expected_revision)) throw new DesktopProblem('invalid');
      const api = getBridge();
      if (!api.SaveAutostart) throw new DesktopProblem('unsupported');
      const raw = await call(() => api.SaveAutostart(input), true);
      try {
        const out = checkedAutostart(raw);
        if (!out.supported || out.enabled !== input.enabled || (input.enabled && !out.matches_current)) throw new Error();
        return out;
      } catch { throw new DesktopProblem('unknown'); }
    },
  };
}
export const autostartService = createAutostartService();
export function autostartError(e: unknown): string {
  const code = e instanceof DesktopProblem ? e.code : 'read_error';
  return code === 'conflict' ? '设置已变化，请重新读取。' : code === 'unknown' ? '结果未知，请重新读取后再操作。' : code === 'permission' ? '没有修改登录项的权限。' : code === 'invalid' ? '本机核心尚未初始化，或桌面启动器路径不可用。' : code === 'unsupported' ? '当前宿主不支持此设置。' : '无法读取或修改本机登录项。';
}
