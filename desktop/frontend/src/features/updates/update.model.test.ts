import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/updates.json';
import { CONTRACT, type DesktopBridge, type UpdateInfo } from '../../desktop/contract.generated';
import { checkedUpdate, UpdateModel } from './update.model';

const info = fixture.downloaded;
function api(overrides: Partial<DesktopBridge> = {}): DesktopBridge {
  const result = { contract: CONTRACT, data: info, problem: null };
  return { UpdateStatus: async () => result, CheckUpdate: async () => result,
    DownloadUpdate: async () => result, InstallUpdate: async () => result, ...overrides } as DesktopBridge;
}
describe('local typed update model (no native installer)', () => {
  it('shares the canonical DTO and picks only public fields', async () => {
    expect(fixture.contract).toBe(CONTRACT);
    expect(checkedUpdate({ ...info, internal_path: 'not public' } as UpdateInfo)).toEqual(info);
    expect(await new UpdateModel(() => api()).status()).toEqual(info);
  });
  it('passes only selected channel/version and explicit approval', async () => {
    const check = vi.fn(api().CheckUpdate);
    const download = vi.fn(api().DownloadUpdate);
    const install = vi.fn(api().InstallUpdate);
    const model = new UpdateModel(() => api({ CheckUpdate: check, DownloadUpdate: download, InstallUpdate: install }));
    await model.check('dev'); await model.download(info.latest_version); await model.install(info.latest_version);
    expect(check).toHaveBeenCalledWith('dev'); expect(download).toHaveBeenCalledWith(info.latest_version);
    expect(install).toHaveBeenCalledWith(fixture.confirmation);
  });
  it.each([
    { phase: 'installed' }, { channel: 'nightly' }, { install_kind: 'shell' },
    { current_version: '' }, { available: 'yes' }, { download_path: '' }, { latest_version: '' },
  ])('rejects malformed update DTO %j', change => {
    expect(() => checkedUpdate({ ...info, ...change } as UpdateInfo)).toThrow('read_error');
  });
  it('does not retry integrity failures or expose remote diagnostics', async () => {
    const install = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'integrity' as const } }));
    await expect(new UpdateModel(() => api({ InstallUpdate: install })).install(info.latest_version)).rejects.toMatchObject({ code: 'integrity' });
    expect(install).toHaveBeenCalledTimes(1);
  });
  it('pins remote checks and confirmation to the selected instance', async () => {
    const check = vi.fn(async () => ({ contract: CONTRACT, data: fixture.remote_checked, problem: null }));
    const install = vi.fn(async () => ({ contract: CONTRACT, data: fixture.remote_applied, problem: null }));
    const model = new UpdateModel(() => api({ CheckRemoteUpdate: check, InstallRemoteUpdate: install }));
    await model.remoteCheck('fixture-remote');
    expect((await model.remoteInstall('fixture-remote', info.latest_version)).phase).toBe('applied');
    expect(check).toHaveBeenCalledWith('fixture-remote');
    expect(install).toHaveBeenCalledWith(fixture.remote_confirmation);
    expect(checkedUpdate(fixture.runtime_downloaded).install_kind).toBe('runtime');
  });
  it('does not retry an unknown remote update', async () => {
    const install = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'unknown' as const } }));
    await expect(new UpdateModel(() => api({ InstallRemoteUpdate: install })).remoteInstall('fixture-remote', info.latest_version)).rejects.toMatchObject({ code: 'unknown' });
    expect(install).toHaveBeenCalledTimes(1);
  });
  it('does not use a mock if the host is missing', async () => {
    await expect(new UpdateModel(() => { throw new Error('missing host'); }).status()).rejects.toMatchObject({ code: 'offline' });
  });
});
