import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/api-service.json';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
import { apiInteger, checkedAPIService, createAPIService } from './api-service.model';

type API = Pick<DesktopBridge, 'APIService' | 'SaveAPIService' | 'IssueAPIKey' | 'RevokeAPIKey'>;
function host(overrides: Partial<API>): API {
  const unavailable = async (): Promise<never> => { throw new Error('unexpected call'); };
  return { APIService: unavailable, SaveAPIService: unavailable, IssueAPIKey: unavailable, RevokeAPIKey: unavailable, ...overrides };
}

describe('application API settings and secret boundary', () => {
  it('uses the shared disabled default and rejects malformed addresses and metadata', () => {
    expect(checkedAPIService(fixture)).toEqual(fixture);
    expect(() => checkedAPIService({ ...fixture, settings: { ...fixture.settings, port: 0 } })).toThrow();
    expect(() => checkedAPIService({ ...fixture, base_urls: ['http://example.invalid/v1'] })).toThrow();
    const key = { id: 'id', label: 'phone', scope: 'gateway', expires_at: 0, revoked: false, token: 'not-for-metadata' };
    expect(checkedAPIService({ ...fixture, keys: [key] }).keys[0]).not.toHaveProperty('token');
    expect(apiInteger('65535', 65535, 1)).toBe(65535);
    for (const text of ['0', '65536', '1.5', '', '1e3']) expect(apiInteger(text, 65535, 1)).toBeNull();
  });
  it('treats malformed write responses as unknown without retrying', async () => {
    let writes = 0;
    const service = createAPIService(() => host({ SaveAPIService: async () => { writes++; return { contract: CONTRACT, problem: null, data: fixture }; } }));
    await expect(service.save({ enabled: true, port: 17862, allow_lan: false, expected_revision: 1 })).rejects.toMatchObject({ code: 'unknown' });
    expect(writes).toBe(1);
  });
  it('returns a new secret once and preserves lost-result uncertainty', async () => {
    const access = { id: 'key-id', label: 'phone', scope: 'gateway', expires_at: 0, revoked: false };
    const service = createAPIService(() => host({ IssueAPIKey: async () => ({ contract: CONTRACT, problem: null, data: { access, token: 'a'.repeat(43) } }) }));
    expect(await service.issue({ label: 'phone', ttl_seconds: 0 })).toEqual({ access, token: 'a'.repeat(43) });
    const failed = createAPIService(() => host({ IssueAPIKey: async () => { throw new Error('network'); } }));
    await expect(failed.issue({ label: 'phone', ttl_seconds: 0 })).rejects.toMatchObject({ code: 'unknown' });
  });
});
