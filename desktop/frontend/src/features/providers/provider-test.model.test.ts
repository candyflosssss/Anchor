import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-probe.json';
import catalog from '../../../../contract/provider-models.json';
import { checkedProgress, modelsForKey } from './provider-test.model';

describe('Key-scoped inference diagnostics', () => {
  it('offers only models returned for the selected Key', () => {
    const value = { ...catalog.success, entries: [{ id: 'a', key_ids: ['first'] }, { id: 'b', key_ids: ['second'] }, { id: 'shared', key_ids: ['first', 'second'] }] };
    expect(modelsForKey(value, 'first')).toEqual(['a', 'shared']);
    expect(modelsForKey(value, 'second')).toEqual(['b', 'shared']);
    expect(modelsForKey(value, 'disabled')).toEqual([]);
  });
  it('uses the shared progress fixture and rejects cross-provider, stale and oversized results', () => {
    expect(checkedProgress(fixture.progress, fixture.request).text).toBe('O');
    for (const value of [{ ...fixture.progress, provider_id: 'other' }, { ...fixture.progress, revision: 4 },
      { ...fixture.progress, key_id: 'other' }, { ...fixture.progress, text: 'x'.repeat(16385) },
      { ...fixture.progress, status: 'completed' }, { ...fixture.progress, elapsed_ms: -1 }]) {
      expect(() => checkedProgress(value, fixture.request)).toThrow('read_error');
    }
    expect(() => checkedProgress(fixture.progress, { ...fixture.request, action: 'read', run_id: 'another-run' })).toThrow('read_error');
  });
});
