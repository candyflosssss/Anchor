import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/model-speed-variants.json';
import { actionFor, checkedWorkspace, isValidModelAlias } from './provider-features.model';
import { MockProviderTransport } from './provider.mock';
import { isValidId } from '../../lib/identifier';

describe('explicit Anchor speed variants', () => {
  it('accepts only model-specific suffixes and full 128 code point limit', () => {
    for (const id of ['gpt-6.1-sol(fast)', '中文(ultrafast)', `${'鱼'.repeat(122)}(fast)`]) {
      expect(isValidModelAlias(id)).toBe(true);
      expect(isValidId(id)).toBe(false);
    }
    for (const id of ['model(fast)(ultrafast)', 'model(other)', `${'鱼'.repeat(123)}(fast)`]) expect(isValidModelAlias(id)).toBe(false);
  });
  it('uses the shared fixture to create independent, explicit speed mappings', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const p = (await transport.list(signal))[0];
    const w = checkedWorkspace(await transport.action(actionFor(p, 'model-put', { ...fixture.action, provider_id: p.id, expected_revision: p.revision }), signal));
    const base = w.models.find(m => m.id === fixture.action.target)!;
    expect(base.service_tier).toBeUndefined();
    for (const expected of fixture.variants) {
      expect(w.models.find(m => m.id === expected.id)).toMatchObject({ ...expected, revision: 1, candidate_count: 1, tools: true, streaming: true });
    }
    expect(() => checkedWorkspace({ ...w, models: [{ ...base, service_tier: 'fake' }] })).toThrow();
    // A collision rejects before even updating the ordinary mapping.
    await expect(transport.action(actionFor(p, 'model-put', { target: base.id, model: 'changed', model_revision: 1, speed_variants: ['fast', 'ultrafast'] }), signal)).rejects.toMatchObject({ code: 'conflict' });
    const after = await transport.workspace(p.id, signal);
    expect(after.models.find(m => m.id === base.id)?.upstream).toBe(base.upstream);
  });
  it('preserves a fixed tier when an old editor omits it', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const p = (await transport.list(signal))[0];
    await transport.action(actionFor(p, 'model-put', { target: 'fixed', model: 'actual', service_tier: 'priority', tools: true, streaming: true }), signal);
    const w = await transport.action(actionFor(p, 'model-put', { target: 'fixed', model: 'actual', model_revision: 1, tools: true, streaming: true }), signal);
    expect(w.models.find(m => m.id === 'fixed')?.service_tier).toBe('priority');
  });
});
