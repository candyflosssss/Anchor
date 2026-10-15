import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-probe.json';
import workspace from '../../../../contract/provider-workspace.json';
import catalog from '../../../../contract/provider-models.json';
import { checkedProgress, keyForMappingTest, modelsForKey, probeFailureText } from './provider-test.model';

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

it('accepts the shared HTML diagnostic and explains HTTP 200 with a webpage body', () => {
  const result = checkedProgress(fixture.html_failure, fixture.request);
  expect(result.result).toMatchObject({ http_status: 200, failure: 'unexpected_html' });
  expect(probeFailureText(result.result!.failure)).toContain('/v1');
  expect(probeFailureText(result.result!.failure)).toContain('HTML');
  expect(probeFailureText('timeout')).toBe('timeout');
});

describe('configured mapping test Key', () => {
  const w = { ...workspace, keys: [
    { ...workspace.keys[0], id: 'unrelated', enabled: true },
    { ...workspace.keys[0], id: 'disabled', enabled: false },
    { ...workspace.keys[0], id: 'bound', enabled: true },
    { ...workspace.keys[0], id: 'alternate', enabled: true },
  ] };
  it('selects an enabled bound Key in binding order, never an unrelated Key', () => {
    expect(keyForMappingTest(w, ['disabled', 'bound'])).toBe('bound');
    expect(keyForMappingTest(w, ['alternate', 'bound'])).toBe('alternate');
    expect(keyForMappingTest(w, ['missing', 'disabled'])).toBe('');
    expect(keyForMappingTest(w, ['anonymous'])).toBe('');
  });
  it('uses an enabled provider Key when the mapping is unrestricted', () => {
    expect(keyForMappingTest(w, [])).toBe('unrelated');
    expect(keyForMappingTest({ ...w, keys: w.keys.map(k => ({ ...k, enabled: false })) }, [])).toBe('');
  });
  it('permits anonymous only without stored Keys and without a missing real binding', () => {
    expect(keyForMappingTest({ ...w, keys: [] }, [])).toBe('anonymous');
    expect(keyForMappingTest({ ...w, keys: [] }, ['anonymous'])).toBe('anonymous');
    expect(keyForMappingTest({ ...w, keys: [] }, ['missing'])).toBe('');
  });
});
