import { describe, expect, it } from 'vitest';
import { actionFor, type Action } from './provider-features.model';
import { MockProviderTransport } from './provider.mock';
import { newDraft, toSave } from './provider.model';

const signal = new AbortController().signal;
async function setup() {
  const transport = new MockProviderTransport(); transport.reset('empty');
  for (const id of ['p', 'other']) {
    await transport.save(toSave({ ...newDraft(), id, base_urls: ['https://example.invalid/v1'], secret: 'synthetic-key' }), signal);
  }
  const act = async (id: string, action: Action['action'], fields: Partial<Action>) => {
    const w = await transport.workspace(id, signal);
    return transport.action(actionFor({ id, revision: w.revision }, action, fields), signal);
  };
  return { transport, act };
}

describe('Key deletion in the memory prototype', () => {
  it('prunes a shared Key list and deletes only orphaned mappings', async () => {
    const { transport, act } = await setup();
    await act('p', 'key-put', { target: 'backup', secret: 'synthetic-key', enabled: true });
    await act('p', 'model-put', { target: 'shared', model: 'same', key_ids: ['default', 'backup'] });
    await act('p', 'model-put', { target: 'only', model: 'same', key_ids: ['default'] });
    const w = await act('p', 'key-delete', { target: 'default', approved: true });
    expect(w.keys.map(k => k.id)).toEqual(['backup']);
    expect(w.models).toHaveLength(1);
    expect(w.models[0]).toMatchObject({ id: 'shared', key_ids: ['backup'], revision: 2 });
    expect((await transport.workspace('p', signal)).models).toEqual(w.models);
  });
  it('preserves another provider using the same model name and Key ID', async () => {
    const { transport, act } = await setup();
    await act('p', 'model-put', { target: 'shared', model: 'same', key_ids: ['default'], tools: true });
    await act('other', 'model-put', { target: 'shared', model: 'same', key_ids: ['default'] });
    const w = await act('p', 'key-delete', { target: 'default', approved: true });
    expect(w.models).toEqual([]);
    const other = await transport.workspace('other', signal);
    expect(other.keys.map(k => k.id)).toEqual(['default']);
    expect(other.models[0]).toMatchObject({ id: 'shared', upstream: 'same', key_ids: ['default'], revision: 3, candidate_count: 1, candidate_index: 0, tools: true });
  });
  it('clears default credentials through the same source cleanup', async () => {
    const { transport, act } = await setup();
    await act('p', 'model-put', { target: 'only', model: 'same', key_ids: ['default'] });
    const p = (await transport.list(signal)).find(p => p.id === 'p')!;
    await transport.save(toSave({ ...newDraft(p), secretMode: 'clear', confirmClear: true }, p), signal);
    const w = await transport.workspace('p', signal);
    expect(w.keys).toEqual([]); expect(w.models).toEqual([]);
  });
  it('removes an all-Keys mapping when its final saved Key is deleted', async () => {
    const { act } = await setup();
    await act('p', 'model-put', { target: 'all', model: 'same', key_ids: [] });
    const w = await act('p', 'key-delete', { target: 'default', approved: true });
    expect(w.models).toEqual([]);
  });
});
