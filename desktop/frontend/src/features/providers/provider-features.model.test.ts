import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-workspace.json';
import { actionFor, checkedWorkspace, type Workspace } from './provider-features.model';
import { MockProviderTransport } from './provider.mock';
describe('provider first priority contract', () => {
  it('reads the shared fixture and strips secrets from public DTOs', () => {
    const value = { ...structuredClone(fixture), secret: 'synthetic-secret', keys: fixture.keys.map(k => ({ ...k, secret: 'synthetic-secret' })) };
    expect(checkedWorkspace(value)).toEqual(fixture);
    expect(JSON.stringify(checkedWorkspace(value))).not.toContain('synthetic-secret');
    expect(() => checkedWorkspace({ ...fixture, prices: [{ model: 'x', input_usd: -1 }] } as unknown as Workspace)).toThrow();
  });
  it('round trips key and mapping writes through the memory prototype', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const p = (await transport.list(signal))[0];
    const w = await transport.action(actionFor(p, 'key-put', { target: 'second', secret: 'synthetic', enabled: true }), signal);
    expect(checkedWorkspace(w).keys).toHaveLength(2);
    expect(JSON.stringify(w)).not.toContain('synthetic');
    await expect(transport.action(actionFor(p, 'strategy', { strategy: 'priority' }), signal)).rejects.toMatchObject({ code: 'conflict' });
  });
});
