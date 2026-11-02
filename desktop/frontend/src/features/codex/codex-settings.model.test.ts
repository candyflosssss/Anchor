import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/codex-settings.json';
import { CONTRACT } from '../../desktop/contract.generated';
import { checkedCodexSettings, createCodexSettingsService, parseCompactPercent, parseRetries } from './codex-settings.model';

describe('unified Codex settings', () => {
  it('uses the shared default and validates the supported range without inventing missing settings', () => {
    expect(checkedCodexSettings(fixture).settings.retries).toBe(5);
    expect(checkedCodexSettings(fixture).settings.auto_compact).toBe(true);
    expect(checkedCodexSettings(fixture).settings.compact_percent).toBe(90);
    for (const n of [1, 90, 99]) expect(parseCompactPercent(String(n))).toBe(n);
    for (const value of ["", "0", "100", "90.5", "NaN"]) expect(parseCompactPercent(value)).toBeNull();
    for (const n of [0, 5, 10]) expect(parseRetries(String(n))).toBe(n);
    for (const value of ['', '-1', '11', '1.5', 'NaN', ' ']) expect(parseRetries(value)).toBeNull();
    for (const value of [null, {}, { ...fixture, settings: { retries: null, revision: 1 } }, { ...fixture, settings: { retries: 5, revision: 0 } }, { ...fixture, needs_sync: [null] }]) expect(() => checkedCodexSettings(value)).toThrow();
  });
  it('saves explicit zero once and treats ignored settings as an uncertain write', async () => {
    const save = vi.fn(async () => ({ contract: CONTRACT, data: { settings: { ...fixture.settings, retries: 0, revision: 2 }, needs_sync: [] }, problem: null }));
    const service = createCodexSettingsService(() => ({ SaveCodexSettings: save, CodexSettings: vi.fn() }));
    const input = { retries: 0, auto_compact: null, compact_percent: null, expected_revision: 1 };
    expect((await service.save(input)).settings.retries).toBe(0);
    expect(save).toHaveBeenCalledExactlyOnceWith(input);
    for (const retries of [null, -1, 11, 0.5, NaN]) await expect(service.save({ ...input, retries })).rejects.toMatchObject({ code: 'invalid' });
    expect(save).toHaveBeenCalledTimes(1);
    save.mockResolvedValue({ contract: CONTRACT, data: { settings: { ...fixture.settings, retries: 5, revision: 2 }, needs_sync: [] }, problem: null });
    await expect(service.save(input)).rejects.toMatchObject({ code: 'unknown' });
    expect(save).toHaveBeenCalledTimes(2);
  });
  it('saves compaction independently and detects an ignored automatic setting', async () => {
    const save = vi.fn(async () => ({ contract: CONTRACT, data: { ...fixture, settings: { ...fixture.settings, auto_compact: false, compact_percent: 80, revision: 2 } }, problem: null }));
    const service = createCodexSettingsService(() => ({ SaveCodexSettings: save, CodexSettings: vi.fn() }));
    const input = { retries: null, auto_compact: false, compact_percent: 80, expected_revision: 1 };
    expect((await service.save(input)).settings.compact_percent).toBe(80);
    for (const percent of [0, 100, 1.5, NaN]) await expect(service.save({ ...input, compact_percent: percent })).rejects.toMatchObject({ code: 'invalid' });
    expect(save).toHaveBeenCalledTimes(1);
    save.mockResolvedValue({ contract: CONTRACT, data: { ...fixture, settings: { ...fixture.settings, revision: 2 } }, problem: null });
    await expect(service.save(input)).rejects.toMatchObject({ code: 'unknown' });
  });
  it('preserves partial sync status and does not retry management errors', async () => {
    const service = createCodexSettingsService(() => ({ CodexSettings: vi.fn(async () => ({ contract: CONTRACT, data: { ...fixture, needs_sync: ['codex-work'] }, problem: null })), SaveCodexSettings: vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'conflict' as const } })) }));
    expect((await service.read()).needs_sync).toEqual(['codex-work']);
    await expect(service.save({ retries: 3, auto_compact: null, compact_percent: null, expected_revision: 1 })).rejects.toMatchObject({ code: 'conflict' as const });
  });
});
