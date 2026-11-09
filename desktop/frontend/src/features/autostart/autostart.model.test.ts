import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/autostart.json';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
import { checkedAutostart, createAutostartService } from './autostart.model';

describe('host-local autostart contract', () => {
  it('validates the shared fixture and unsupported state', () => {
    expect(checkedAutostart(fixture.data)).toEqual(fixture.data);
    expect(checkedAutostart({ supported: false, enabled: false, matches_current: false, disabled_by_system: false, can_enable: false, revision: '' }).supported).toBe(false);
    for (const v of [null, {}, { ...fixture.data, revision: '' }, { ...fixture.data, supported: false }, { ...fixture.data, enabled: null }, { ...fixture.data, can_enable: undefined }, { ...fixture.data, matches_current: true }]) expect(() => checkedAutostart(v)).toThrow();
  });
  it('requires approval and checks the exact write outcome', async () => {
    const out = { ...fixture.data, enabled: true, matches_current: true };
    const save = vi.fn(async () => ({ contract: CONTRACT, data: out, problem: null }));
    const service = createAutostartService(() => ({ Autostart: vi.fn(), SaveAutostart: save }));
    const input = { enabled: true, approved: true, expected_revision: fixture.data.revision };
    await expect(service.save({ ...input, approved: false })).rejects.toMatchObject({ code: 'invalid' });
    await expect(service.save({ ...input, enabled: null })).rejects.toMatchObject({ code: 'invalid' });
    expect(save).not.toHaveBeenCalled();
    expect(await service.save(input)).toEqual(out);
    expect(save).toHaveBeenCalledExactlyOnceWith(input);
    save.mockResolvedValue({ contract: CONTRACT, data: { ...out, matches_current: false }, problem: null });
    await expect(service.save(input)).rejects.toMatchObject({ code: 'unknown' });
    expect(save).toHaveBeenCalledTimes(2);
  });
  it('does not retry conflicts or use mock data for old hosts', async () => {
    const save = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'conflict' as const } }));
    const service = createAutostartService(() => ({ Autostart: vi.fn(), SaveAutostart: save }));
    await expect(service.save({ enabled: false, approved: true, expected_revision: fixture.data.revision })).rejects.toMatchObject({ code: 'conflict' });
    expect(save).toHaveBeenCalledTimes(1);
    await expect(createAutostartService(() => ({} as DesktopBridge)).read()).rejects.toMatchObject({ code: 'unsupported' });
  });
});
