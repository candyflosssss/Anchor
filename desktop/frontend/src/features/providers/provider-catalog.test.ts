import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-models.json';
import { checkedCatalog, catalogProblemText } from './provider.model';
import { MockProviderTransport } from './provider.mock';
import { actionFor } from './provider-features.model';
describe('read-only catalog model', () => {
  it.each([fixture.success, fixture.empty, fixture.unsupported])('accepts the shared result %#', value => {
    expect(checkedCatalog(value, fixture.input)).toEqual(value);
  });
  it.each([
    { revision: 4 }, { key_id: 'other' }, { key_id: null }, { models: null }, { failure: 'raw-secret' }, { elapsed_ms: -1 },
    { http_status: 0 }, { models: ['duplicate', 'duplicate'] }, { models: ['bad\nname'] }, { models: ['bad\u202ename'] },
    { models: ['x'.repeat(257)] }, { failure: 'unsupported', models: ['partial'] },
  ])('fails closed on invalid result %#', patch => {
    expect(() => checkedCatalog({ ...fixture.success, ...patch } as typeof fixture.success, fixture.input)).toThrow('read_error');
  });
  it.each([fixture.aggregate, fixture.partial, fixture.no_enabled_keys])('accepts merged and partial results %#', value => {
    expect(checkedCatalog(value, fixture.aggregate_input)).toEqual(value);
  });
  it.each([
    { entries: [{ id: 'demo-model', key_ids: ['missing'] }, fixture.success.entries[1]] },
    { entries: [{ id: 'demo-model', key_ids: ['default', 'default'] }, fixture.success.entries[1]] },
    { keys: [{ ...fixture.success.keys[0], multiplier: '-1' }] },
    { keys: [{ ...fixture.success.keys[0], multiplier: 'Infinity' }] },
    { keys: [{ ...fixture.success.keys[0], failure: 'authentication', http_status: 401 }] },
    { partial: true }, { entries: null }, { keys: null },
  ])('rejects invalid associations and rates %#', patch => {
    expect(() => checkedCatalog({ ...fixture.success, ...patch } as typeof fixture.success, fixture.input)).toThrow('read_error');
  });
  it('keeps zero multipliers and clones key associations', () => {
    const value = { ...fixture.success, keys: [{ ...fixture.success.keys[0], multiplier: '0' }] };
    const result = checkedCatalog(value, fixture.input);
    expect(result.keys[0].multiplier).toBe('0');
    result.entries[0].key_ids.push('other');
    expect(value.entries[0].key_ids).toEqual(['default']);
  });
  it('mock skips disabled keys and preserves all enabled associations and rates', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const provider = (await transport.list(signal))[0];
    let w = await transport.action(actionFor(provider, 'key-put', { target: 'second', secret: 'synthetic-second', enabled: true, protocol: 'anthropic' }), signal);
    w = await transport.action(actionFor({ ...provider, revision: w.revision }, 'key-put', { target: 'disabled', secret: 'synthetic-disabled', enabled: false }), signal);
    let input = { provider_id: provider.id, expected_revision: w.revision, key_id: '' };
    const merged = checkedCatalog(await transport.discover(input, signal), input);
    expect(merged.keys.map(k => k.id)).toEqual(['default', 'second']);
    expect(merged.entries.every(e => e.key_ids.join(',') === 'default,second')).toBe(true);
    expect(merged.keys.map(k => k.multiplier)).toEqual(['0.8', '1.5']);
    expect(checkedCatalog(await transport.discover({ ...input, key_id: 'disabled' }, signal), { ...input, key_id: 'disabled' }).failure).toBe('no_enabled_keys');
    for (const target of ['default', 'second']) {
      w = await transport.action(actionFor({ ...provider, revision: w.revision }, 'key-put', { target, enabled: false }), signal);
    }
    input = { ...input, expected_revision: w.revision };
    const skipped = checkedCatalog(await transport.discover(input, signal), input);
    expect(skipped).toMatchObject({ failure: 'no_enabled_keys', http_status: 0, models: [], entries: [], keys: [], partial: false });
  });
  it('does not describe a read error as an uncertain write', () => {
    expect(catalogProblemText('unknown')).toContain('配置未改变');
    expect(catalogProblemText('unknown')).not.toContain('可能已经保存');
  });
  it('mock discovery uses the shared fixture without saving a revision', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const before = await transport.list(signal);
    const selected = before[0];
    expect(await transport.discover({ provider_id: selected.id, expected_revision: selected.revision, key_id: 'default' }, signal)).toEqual({ ...fixture.success, provider_id: selected.id, revision: selected.revision });
    const all = await transport.discover({ provider_id: selected.id, expected_revision: selected.revision, key_id: '' }, signal);
    expect(all.key_id).toBe('');
    expect(all.entries).toEqual(fixture.success.entries);
    expect(await transport.list(signal)).toEqual(before);
    await expect(transport.discover({ provider_id: selected.id, expected_revision: selected.revision + 1, key_id: 'default' }, signal)).rejects.toMatchObject({ code: 'conflict' });
  });
});
