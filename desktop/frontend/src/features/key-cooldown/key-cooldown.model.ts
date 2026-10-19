import { bridge, call, DesktopProblem } from '../../desktop/transport';
import type { DesktopBridge, KeyCooldownSettings, SaveKeyCooldownSettings } from '../../desktop/contract.generated';

export function parseCooldownInteger(value: string, max: number): number | null {
  return /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) >= 1 && Number(value) <= max ? Number(value) : null;
}
export function checkedKeyCooldown(value: unknown): KeyCooldownSettings {
  const v = value as KeyCooldownSettings | null;
  if (!v || typeof v.enabled !== 'boolean' || !Number.isSafeInteger(v.revision) || v.revision < 1 || v.revision > Number.MAX_SAFE_INTEGER ||
    !Number.isInteger(v.failure_threshold) || v.failure_threshold < 1 || v.failure_threshold > 100 ||
    !Number.isInteger(v.cooldown_seconds) || v.cooldown_seconds < 1 || v.cooldown_seconds > 3600 ||
    !Number.isInteger(v.rate_limit_seconds) || v.rate_limit_seconds < 1 || v.rate_limit_seconds > 3600) throw new DesktopProblem('read_error');
  return { enabled: v.enabled, failure_threshold: v.failure_threshold, cooldown_seconds: v.cooldown_seconds, rate_limit_seconds: v.rate_limit_seconds, revision: v.revision };
}
export function createKeyCooldownService(getBridge: () => Pick<DesktopBridge, 'KeyCooldownSettings' | 'SaveKeyCooldownSettings'> = bridge) {
  return {
    async read() {
      const api = getBridge();
      if (!api.KeyCooldownSettings) throw new DesktopProblem('unsupported');
      return checkedKeyCooldown(await call(() => api.KeyCooldownSettings()));
    },
    async save(input: SaveKeyCooldownSettings) {
      try { checkedKeyCooldown({ ...input, revision: input.expected_revision }); }
      catch { throw new DesktopProblem('invalid'); }
      if (input.expected_revision >= Number.MAX_SAFE_INTEGER) throw new DesktopProblem('invalid');
      const api = getBridge();
      if (!api.SaveKeyCooldownSettings) throw new DesktopProblem('unsupported');
      const raw = await call(() => api.SaveKeyCooldownSettings(input), true);
      try {
        const out = checkedKeyCooldown(raw);
        if (out.revision !== input.expected_revision + 1 || out.enabled !== input.enabled || out.failure_threshold !== input.failure_threshold || out.cooldown_seconds !== input.cooldown_seconds || out.rate_limit_seconds !== input.rate_limit_seconds) throw new Error();
        return out;
      } catch { throw new DesktopProblem('unknown'); }
    },
  };
}
export const keyCooldownService = createKeyCooldownService();
export function keyCooldownError(error: unknown): string {
  const code = error instanceof DesktopProblem ? error.code : 'read_error';
  return code === 'conflict' ? '版本冲突' : code === 'unknown' ? '结果未知' : code === 'unsupported' ? '不支持' : code === 'invalid' ? '参数无效' : code === 'unauthorized' ? '未授权' : '连接异常';
}
