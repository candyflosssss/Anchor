import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { CodexCompactionStatus, CodexSettingsState, DesktopBridge, SaveCodexSettings } from '../../desktop/contract.generated';

export const parseRetries = (value: string): number | null => /^\d+$/.test(value) && Number(value) <= 10 ? Number(value) : null;
export const parseCompactPercent = (value: string): number | null => /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= 99 ? Number(value) : null;
const compactionStatuses = new Set(['written', 'missing_context', 'needs_sync', 'unmanaged', 'unsupported', 'disabled']);
function checkedCompaction(row: CodexCompactionStatus): boolean {
  const positive = (v: number | null) => v === null || (Number.isSafeInteger(v) && v > 0);
  if (!row || typeof row.binding_id !== 'string' || !row.binding_id || typeof row.model_id !== 'string' || !row.model_id || typeof row.is_default !== 'boolean' || !compactionStatuses.has(row.status) || !positive(row.context_window) || !positive(row.token_limit)) return false;
  if (row.status === 'written') return row.context_window !== null && row.token_limit !== null;
  if (row.status === 'disabled') return row.token_limit === null;
  return row.context_window === null && row.token_limit === null;
}
export function codexSettingsNotice(state: CodexSettingsState): string {
  if (state.needs_sync.length) return '设置已保存；部分配置待同步';
  if (state.settings.auto_compact && state.compaction.some(row => row.status !== 'written')) return '设置已保存；自动压缩配置未完成，请处理下方提示';
  if (state.settings.auto_compact && !state.compaction.length) return '设置已保存；尚无已启用的 Codex 托管目标';
  return '设置已保存；待重启 Codex';
}
export function checkedCodexSettings(value: unknown): CodexSettingsState {
  const v = value as CodexSettingsState | null;
  if (!v || !v.settings || !Number.isSafeInteger(v.settings.revision) || v.settings.revision < 1 || !Number.isInteger(v.settings.retries) || v.settings.retries < 0 || v.settings.retries > 10 || typeof v.settings.auto_compact !== 'boolean' || !Number.isInteger(v.settings.compact_percent) || v.settings.compact_percent < 1 || v.settings.compact_percent > 99 || !Array.isArray(v.needs_sync) || v.needs_sync.some(id => typeof id !== 'string' || !id.length)) throw new DesktopProblem('read_error');
  // An older core cannot report whether a model actually has a threshold.
  // Reject that response instead of presenting an empty list as healthy.
  if (v.compaction === undefined) throw new DesktopProblem('unsupported');
  if (!Array.isArray(v.compaction) || !v.compaction.every(checkedCompaction)) throw new DesktopProblem('read_error');
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
      if ((input.retries == null && input.auto_compact == null && input.compact_percent == null) || (input.retries != null && (!Number.isInteger(input.retries) || input.retries < 0 || input.retries > 10)) || (input.auto_compact != null && typeof input.auto_compact !== 'boolean') || (input.compact_percent != null && (!Number.isInteger(input.compact_percent) || input.compact_percent < 1 || input.compact_percent > 99)) || !Number.isSafeInteger(input.expected_revision) || input.expected_revision < 1 || input.expected_revision >= Number.MAX_SAFE_INTEGER) throw new DesktopProblem('invalid');
      const api = getBridge();
      if (!api.SaveCodexSettings) throw new DesktopProblem('unsupported');
      const raw = await call(() => api.SaveCodexSettings(input), true);
      try {
        const out = checkedCodexSettings(raw);
        if ((input.retries != null && out.settings.retries !== input.retries) || (input.auto_compact != null && out.settings.auto_compact !== input.auto_compact) || (input.compact_percent != null && out.settings.compact_percent !== input.compact_percent) || out.settings.revision !== input.expected_revision + 1) throw new Error();
        return out;
      } catch { throw new DesktopProblem('unknown'); }
    },
  };
}
export const codexSettingsService = createCodexSettingsService();
export function codexSettingsError(error: unknown) {
  const code = error instanceof DesktopProblem ? error.code : 'read_error';
  return code === 'conflict' ? '版本冲突' : code === 'unknown' ? '结果未知' : code === 'unsupported' ? '当前核心不支持配置检查，请同步升级 Anchor 核心与桌面' : code === 'invalid' ? '参数无效' : '连接异常';
}
