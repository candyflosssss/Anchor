import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { CodexSettingsState, DesktopBridge, SaveCodexSettings } from '../../desktop/contract.generated';

export const parseRetries = (value: string): number | null => /^\d+$/.test(value) && Number(value) <= 10 ? Number(value) : null;
export function checkedCodexSettings(value: unknown): CodexSettingsState {
  const v = value as CodexSettingsState | null;
  if (!v || !v.settings || !Number.isSafeInteger(v.settings.revision) || v.settings.revision < 1 || !Number.isInteger(v.settings.retries) || v.settings.retries < 0 || v.settings.retries > 10 || !Array.isArray(v.needs_sync) || v.needs_sync.some(id => typeof id !== 'string' || !id.length)) throw new DesktopProblem('read_error');
  return v;
}
export function createCodexSettingsService(getBridge: () => Pick<DesktopBridge, 'CodexSettings' | 'SaveCodexSettings'> = bridge) {
  return {
    async read() {
      const api = getBridge();
      if (!api.CodexSettings) throw new DesktopProblem('unsupported');
      return checkedCodexSettings(await call(() => api.CodexSettings()));
    },
    async save(input: SaveCodexSettings) {
      if (input.retries === null || !Number.isInteger(input.retries) || input.retries < 0 || input.retries > 10 || !Number.isSafeInteger(input.expected_revision) || input.expected_revision < 1 || input.expected_revision >= Number.MAX_SAFE_INTEGER) throw new DesktopProblem('invalid');
      const api = getBridge();
      if (!api.SaveCodexSettings) throw new DesktopProblem('unsupported');
      const raw = await call(() => api.SaveCodexSettings(input), true);
      try {
        const out = checkedCodexSettings(raw);
        if (out.settings.retries !== input.retries || out.settings.revision !== input.expected_revision + 1) throw new Error();
        return out;
      } catch { throw new DesktopProblem('unknown'); }
    },
  };
}
export const codexSettingsService = createCodexSettingsService();
export function codexSettingsError(error: unknown) {
  const code = error instanceof DesktopProblem ? error.code : 'read_error';
  return code === 'conflict' ? '设置已被其他操作修改，请重新读取后编辑。' : code === 'unknown' ? '保存结果未知，请重新读取并核对。' : code === 'unsupported' ? '当前核心不支持 Codex 统一重试设置，请升级核心。' : code === 'invalid' ? '重试次数须为 0–10 的整数。' : '无法读取或保存 Codex 设置，请检查核心连接后重试。';
}
