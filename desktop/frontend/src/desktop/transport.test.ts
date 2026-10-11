import { afterEach, describe, expect, it, vi } from 'vitest';
import catalogFixture from '../../../contract/provider-models.json';
import fixture from '../../../contract/providers.json';
import { CONTRACT, PROBLEM_CODES, type DesktopBridge, type ProvidersResult, type Provider, type SessionResult } from './contract.generated';
import { bridge, call, checkedProvider, checkedSession, DesktopProviderTransport, desktopSession } from './transport';

const signal = () => new AbortController().signal;
const publicRow = fixture.providers[0];
const session = { instance_id: 'synthetic-instance', phase: 'ready', source: 'started', can_stop: true };
const input = { provider: publicRow, expected_revision: publicRow.revision, secret: 'synthetic-secret' };
function testBridge(overrides: Partial<DesktopBridge> = {}): DesktopBridge {
  return {
    Start: vi.fn(async () => ({ contract: CONTRACT, data: session, problem: null })),
    Status: vi.fn(async () => ({ contract: CONTRACT, data: session, problem: null })),
    Logs: vi.fn(), LogDetail: vi.fn(), LogSettings: vi.fn(), ConfigureLogs: vi.fn(), ExportLogs: vi.fn(),
    ProviderWorkspace: vi.fn(), ProviderAction: vi.fn(),
    Providers: vi.fn(async () => ({ contract: CONTRACT, data: fixture.providers, problem: null })),
    DiscoverProviderModels: vi.fn(async () => ({ contract: CONTRACT, data: catalogFixture.success, problem: null })),
    SaveProvider: vi.fn(async () => ({ contract: CONTRACT, data: publicRow, problem: null })),
    ExitDesktop: vi.fn(async () => ({ contract: CONTRACT, data: { accepted: true }, problem: null })),
    StopCore: vi.fn(async () => ({ contract: CONTRACT, data: { accepted: true }, problem: null })),
    UpdateStatus: vi.fn(), CheckUpdate: vi.fn(), DownloadUpdate: vi.fn(), InstallUpdate: vi.fn(),
    ...overrides,
  };
}
afterEach(() => vi.unstubAllGlobals());
describe('restricted desktop transport (not a native Wails test)', () => {
  it('shares the canonical fixture and picks only public fields', async () => {
    expect(fixture.contract).toBe(CONTRACT);
    for (const code of Object.values(fixture.errors)) expect(PROBLEM_CODES).toContain(code.code);
    const api = testBridge();
    const transport = new DesktopProviderTransport(() => api);
    expect(await transport.list(signal())).toEqual(fixture.providers);
    expect(checkedProvider({ ...publicRow, secret: 'never-return-this' } as Provider)).toEqual(publicRow);
    expect(checkedSession({ ...session, token: 'never-return-this' } as typeof session)).toEqual(session);
  });
  it('does not fallback to mock without host bindings', async () => {
    vi.stubGlobal('window', {});
    expect(() => bridge()).toThrow('unsupported');
    await expect(desktopSession.start()).rejects.toMatchObject({ code: 'unsupported' });
    await expect(new DesktopProviderTransport().list(signal())).rejects.toMatchObject({ code: 'read_error' });
  });
  it.each(Object.keys(fixture.errors) as (keyof typeof fixture.errors)[])('maps shared problem %s without exposing raw details', async code => {
    const response = { contract: CONTRACT, data: [], problem: fixture.errors[code] } as ProvidersResult;
    const transport = new DesktopProviderTransport(() => testBridge({ Providers: async () => response }));
    await expect(transport.list(signal())).rejects.toMatchObject({ code, message: code });
  });
  it('treats transport failures after a write as unknown and never replays', async () => {
    const save = vi.fn(async () => { throw new Error('raw-credential-details'); });
    const transport = new DesktopProviderTransport(() => testBridge({ SaveProvider: save }));
    await expect(transport.save(input, signal())).rejects.toMatchObject({ code: 'unknown', message: 'unknown' });
    expect(save).toHaveBeenCalledTimes(1);
  });
  it('validates the contract version and runtime error allowlist', async () => {
    await expect(call(async () => ({ contract: 'other', data: [], problem: null }))).rejects.toMatchObject({ code: 'unsupported' });
    await expect(call(async () => ({ contract: 'other', data: publicRow, problem: null }), true)).rejects.toMatchObject({ code: 'unknown' });
    const response = { contract: CONTRACT, data: null, problem: { code: 'raw-secret' } } as unknown as SessionResult;
    await expect(call(async () => response)).rejects.toMatchObject({ code: 'read_error', message: 'read_error' });
  });
  it.each([0, -1, Number.MAX_SAFE_INTEGER + 1, 1.5])('rejects unsafe provider revision %s', revision => {
    expect(() => checkedProvider({ ...publicRow, revision })).toThrow('read_error');
  });
  it('invalid post-write DTOs and non-accepted shutdowns have unknown outcomes', async () => {
    const api = testBridge({ SaveProvider: async () => ({ contract: CONTRACT, data: { ...publicRow, revision: 0 }, problem: null }), StopCore: async () => ({ contract: CONTRACT, data: { accepted: false }, problem: null }) });
    await expect(new DesktopProviderTransport(() => api).save(input, signal())).rejects.toMatchObject({ code: 'unknown' });
    vi.stubGlobal('window', { go: { desktop: { Bridge: api } } });
    await expect(desktopSession.stop(session)).rejects.toMatchObject({ code: 'unknown' });
    expect(() => checkedSession({ ...session, phase: 'starting' })).toThrow('read_error');
  });
  it('checks cancellation before dispatch and suppresses late read responses', async () => {
    const api = testBridge(); const abort = new AbortController(); abort.abort();
    const transport = new DesktopProviderTransport(() => api);
    await expect(transport.save(input, abort.signal)).rejects.toThrow();
    expect(api.SaveProvider).not.toHaveBeenCalled();
    const readAbort = new AbortController();
    const pending = new DesktopProviderTransport(() => testBridge({ Providers: async () => { readAbort.abort(); return { contract: CONTRACT, data: fixture.providers, problem: null }; } }));
    await expect(pending.list(readAbort.signal)).rejects.toThrow();
  });
});

describe('upstream directory desktop boundary', () => {
  it('uses the shared catalog fixture and drops unexpected fields', async () => {
    const api = testBridge({ DiscoverProviderModels: async () => ({ contract: CONTRACT, data: { ...catalogFixture.success, secret: 'hidden' }, problem: null }) });
    expect(await new DesktopProviderTransport(() => api).discover(catalogFixture.input, signal())).toEqual(catalogFixture.success);
  });
  it('does not classify a directory read as an uncertain save or retry', async () => {
    const operation = vi.fn(async () => ({ contract: CONTRACT, data: null, problem: { code: 'unknown' as const } }));
    await expect(new DesktopProviderTransport(() => testBridge({ DiscoverProviderModels: operation })).discover(catalogFixture.input, signal())).rejects.toMatchObject({ code: 'read_error' });
    expect(operation).toHaveBeenCalledTimes(1);
  });
  it('rejects responses for another saved revision and cancelled results', async () => {
    const operation = vi.fn(async () => ({ contract: CONTRACT, data: { ...catalogFixture.success, revision: 4 }, problem: null }));
    await expect(new DesktopProviderTransport(() => testBridge({ DiscoverProviderModels: operation })).discover(catalogFixture.input, signal())).rejects.toMatchObject({ code: 'read_error' });
    const abort = new AbortController();
    const transport = new DesktopProviderTransport(() => testBridge({ DiscoverProviderModels: async () => { abort.abort(); return { contract: CONTRACT, data: catalogFixture.success, problem: null }; } }));
    await expect(transport.discover(catalogFixture.input, abort.signal)).rejects.toMatchObject({ code: 'read_error' });
  });
});
