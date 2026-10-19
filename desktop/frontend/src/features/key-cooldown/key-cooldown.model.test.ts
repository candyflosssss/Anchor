import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/key-cooldown.json';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
import { checkedKeyCooldown, createKeyCooldownService, parseCooldownInteger } from './key-cooldown.model';

describe('Key cooldown settings contract', () => {
  it('uses the shared default and rejects missing or unsafe fields', () => {
    expect(checkedKeyCooldown(fixture)).toEqual(fixture);
    for (const value of [null, {}, { ...fixture, enabled: null }, { ...fixture, failure_threshold: 0 }, { ...fixture, failure_threshold: 101 }, { ...fixture, cooldown_seconds: 3601 }, { ...fixture, rate_limit_seconds: 0 }, { ...fixture, revision: Number.MAX_SAFE_INTEGER + 1 }]) expect(() => checkedKeyCooldown(value)).toThrow();
    for (const value of ['', ' ', '0', '-1', '1.5', '1e2', 'NaN', '101']) expect(parseCooldownInteger(value, 100)).toBeNull();
    expect(parseCooldownInteger('100', 100)).toBe(100);
    expect(parseCooldownInteger('3600', 3600)).toBe(3600);
  });
  it('saves false exactly once and verifies all returned fields', async () => {
    const result = { ...fixture, enabled: false, revision: 2 };
    const save = vi.fn(async () => ({ contract: CONTRACT, data: result, problem: null }));
    const service = createKeyCooldownService(() => ({ KeyCooldownSettings: vi.fn(), SaveKeyCooldownSettings: save }));
    const input = { enabled: false, failure_threshold: 3, cooldown_seconds: 30, rate_limit_seconds: 30, expected_revision: 1 };
    expect(await service.save(input)).toEqual(result);
    expect(save).toHaveBeenCalledExactlyOnceWith(input);
    for (const field of ['enabled', 'failure_threshold', 'cooldown_seconds', 'rate_limit_seconds', 'expected_revision']) await expect(service.save({ ...input, [field]: null })).rejects.toMatchObject({ code: 'invalid' });
    expect(save).toHaveBeenCalledTimes(1);
    for (const field of ['enabled', 'failure_threshold', 'cooldown_seconds', 'rate_limit_seconds', 'revision'] as const) {
      save.mockResolvedValue({ contract: CONTRACT, data: { ...result, [field]: field === 'enabled' ? true : 1 }, problem: null });
      await expect(service.save(input)).rejects.toMatchObject({ code: 'unknown' });
    }
    expect(save).toHaveBeenCalledTimes(6);
  });
  it('does not retry writes or silently fall back on old hosts', async () => {
    const save = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'conflict' as const } }));
    const service = createKeyCooldownService(() => ({ KeyCooldownSettings: vi.fn(), SaveKeyCooldownSettings: save }));
    await expect(service.save({ enabled: true, failure_threshold: 3, cooldown_seconds: 30, rate_limit_seconds: 30, expected_revision: 1 })).rejects.toMatchObject({ code: 'conflict' });
    expect(save).toHaveBeenCalledTimes(1);
    const unsupported = createKeyCooldownService(() => ({} as DesktopBridge));
    await expect(unsupported.read()).rejects.toMatchObject({ code: 'unsupported' });
  });
});
