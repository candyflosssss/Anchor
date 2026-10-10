import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-models.json';
import { checkedCatalog, catalogProblemText } from './provider.model';
import { MockProviderTransport } from './provider.mock';
describe('read-only catalog model', () => {
  it.each([fixture.success, fixture.empty, fixture.unsupported])('accepts the shared result %#', value => {
    expect(checkedCatalog(value, fixture.input)).toEqual(value);
  });
  it.each([
    { revision: 4 }, { models: null }, { failure: 'raw-secret' }, { elapsed_ms: -1 },
    { http_status: 0 }, { models: ['duplicate', 'duplicate'] }, { models: ['bad\nname'] }, { models: ['bad\u202ename'] },
    { models: ['x'.repeat(257)] }, { failure: 'unsupported', models: ['partial'] },
  ])('fails closed on invalid result %#', patch => {
    expect(() => checkedCatalog({ ...fixture.success, ...patch } as typeof fixture.success, fixture.input)).toThrow('read_error');
  });
  it('does not describe a read error as an uncertain write', () => {
    expect(catalogProblemText('unknown')).toContain('配置未改变');
    expect(catalogProblemText('unknown')).not.toContain('可能已经保存');
  });
  it('mock discovery uses the shared fixture without saving a revision', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const before = await transport.list(signal);
    const selected = before[0];
    expect(await transport.discover({ provider_id: selected.id, expected_revision: selected.revision }, signal)).toEqual({ ...fixture.success, provider_id: selected.id, revision: selected.revision });
    expect(await transport.list(signal)).toEqual(before);
    await expect(transport.discover({ provider_id: selected.id, expected_revision: selected.revision + 1 }, signal)).rejects.toMatchObject({ code: 'conflict' });
  });
});
