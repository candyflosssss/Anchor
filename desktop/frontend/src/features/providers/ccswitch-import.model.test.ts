import { afterEach, describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/ccswitch-import.json';
import { CONTRACT } from '../../desktop/contract.generated';
import { ccImportTransport, importCounts, checkedImportReport } from './ccswitch-import.model';

afterEach(() => vi.unstubAllGlobals());
describe('CC Switch import boundary', () => {
  it('counts only new providers, with Keys across all source platforms', () => {
    expect(importCounts(fixture)).toEqual({ providers: 1, keys: 2, mappings: 1 });
    expect(importCounts({ ...fixture, providers: fixture.providers.map(p => ({ ...p, status: 'exists' })) }).providers).toBe(0);
  });
  it('rejects malformed reports and discards extra credential fields', () => {
    expect(checkedImportReport({ ...fixture, secret: 'synthetic-private' } as typeof fixture)).toEqual(fixture);
    expect(() => checkedImportReport({ ...fixture, providers: [{ ...fixture.providers[0], keys: [{ id: 'k', protocol: 'unknown' }] }] })).toThrow();
  });
  it('submits the reviewed fingerprint and treats a lost write response as unknown', async () => {
    const run = vi.fn(async () => ({ contract: CONTRACT, data: fixture, problem: null }));
    vi.stubGlobal('window', { go: { desktop: { Bridge: { ImportCCSwitch: run } } } });
    const signal = new AbortController().signal;
    const input = { database_path: fixture.database_path, fingerprint: fixture.fingerprint, approved: true };
    expect(await ccImportTransport.run(input, signal)).toEqual(fixture);
    expect(run).toHaveBeenCalledExactlyOnceWith(input);
    run.mockRejectedValueOnce(new Error('private upstream detail'));
    await expect(ccImportTransport.run(input, signal)).rejects.toMatchObject({ code: 'unknown', message: 'unknown' });
    expect(run).toHaveBeenCalledTimes(2);
  });
  it('never opens a fallback inventory when the desktop binding is absent', async () => {
    vi.stubGlobal('window', {});
    await expect(ccImportTransport.run({ database_path: '', fingerprint: '', approved: false }, new AbortController().signal)).rejects.toMatchObject({ code: 'unsupported' });
  });
});
