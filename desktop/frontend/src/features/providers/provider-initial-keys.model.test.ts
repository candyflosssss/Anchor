import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-create.json';
import { MockProviderTransport } from './provider.mock';
import { clearDraftSecrets, clearSaveSecrets, newDraft, nextInitialKey, toSave, validateDraft } from './provider.model';

const draft = () => ({ ...newDraft(), ...{ id: fixture.provider.id, base_urls: [...fixture.provider.base_urls], secret: fixture.secret, initialKeys: structuredClone(fixture.initial_keys) } });
const signal = () => new AbortController().signal;

describe('multiple creation Keys and request addresses', () => {
  it('sends one atomic request matching the shared Go fixture', () => {
    expect(validateDraft(draft())).toEqual({});
    expect(toSave(draft())).toEqual(fixture);
    expect(toSave({ ...draft(), secret: '' }).secret).toBeNull();
  });
  it('validates names, protocols, blank/oversized secrets and every address', () => {
    const value = draft();
    value.initialKeys = [
      { id: 'default', protocol: 'bad', secret: '' },
      { id: 'duplicate', protocol: 'responses', secret: 'line\nbreak' },
      { id: 'duplicate', protocol: 'responses', secret: '密'.repeat(2731) },
      { id: 'bad name', protocol: 'responses', secret: 'synthetic-test' },
    ];
    value.base_urls = ['https://api.example.invalid/v1', 'https://api.example.invalid/v1/', 'http://192.0.2.1/v1'];
    const errors = validateDraft(value);
    for (const field of ['initial_key_0_id', 'initial_key_0_protocol', 'initial_key_0_secret', 'initial_key_1_secret', 'initial_key_2_id', 'initial_key_2_secret', 'initial_key_3_id', 'base_url_1', 'base_url_2']) expect(errors).toHaveProperty(field);
    expect(validateDraft({ ...draft(), initialKeys: Array.from({ length: 64 }, (_, i) => ({ id: `key-${i}`, secret: 'synthetic-test', protocol: 'responses' })) }).initialKeys).toBeDefined();
  });
  it('generates unused names and never sends creation Keys during editing', () => {
    expect(nextInitialKey({ ...draft(), protocol: 'anthropic', initialKeys: [{ id: 'key-2', protocol: 'responses', secret: '' }] })).toEqual({ id: 'key-3', protocol: 'anthropic', secret: '' });
    const source = { ...fixture.provider, revision: 1, has_secret: true };
    expect(newDraft(source).initialKeys).toEqual([]);
    expect(toSave(draft(), source)).not.toHaveProperty('initial_keys');
  });
  it('clears all form secrets and later clears the independent transport snapshot', () => {
    const value = draft(), input = toSave(value), cleared = clearDraftSecrets(value);
    expect(cleared.secret).toBe('');
    expect(cleared.initialKeys.every(key => !key.secret)).toBe(true);
    expect(input.initial_keys?.[0].secret).toBe(fixture.initial_keys[0].secret);
    clearSaveSecrets(input);
    expect(input.secret).toBeNull(); expect(input.initial_keys?.every(key => !key.secret)).toBe(true);
  });
  it('persists every Key independently and preserves them when editing request roots', async () => {
    const transport = new MockProviderTransport();
    const created = await transport.save(toSave(draft()), signal());
    expect(created.revision).toBe(1);
    const keys = (await transport.workspace(created.id, signal())).keys;
    expect(keys.map(key => [key.id, key.protocol])).toEqual([['default', 'responses'], ['backup', 'anthropic'], ['备用', 'chat-completions']]);
    const changed = await transport.save(toSave({ ...newDraft(created), base_urls: ['https://changed.example.invalid/v1', 'https://192.0.2.12/v1'] }, created), signal());
    expect(changed.revision).toBe(2);
    expect((await transport.workspace(created.id, signal())).keys).toEqual(keys);
    expect(JSON.stringify(await transport.workspace(created.id, signal()))).not.toContain('synthetic-create-');
  });
  it('supports named Keys without a default and rejects invalid/duplicate creation atomically', async () => {
    const transport = new MockProviderTransport();
    const invalid = toSave(draft()); invalid.initial_keys![1].id = 'backup';
    await expect(transport.save(invalid, signal())).rejects.toMatchObject({ code: 'invalid' });
    expect((await transport.list(signal())).some(row => row.id === fixture.provider.id)).toBe(false);
    const created = await transport.save(toSave({ ...draft(), secret: '' }), signal());
    expect(created.has_secret).toBe(true);
    expect((await transport.workspace(created.id, signal())).keys.map(key => key.id)).toEqual(['backup', '备用']);
    await expect(transport.save(toSave(draft()), signal())).rejects.toMatchObject({ code: 'conflict' });
  });
  it('does not lose Keys after an unknown write outcome', async () => {
    const transport = new MockProviderTransport(); transport.reset('unknown');
    await expect(transport.save(toSave(draft()), signal())).rejects.toMatchObject({ code: 'unknown' });
    expect((await transport.workspace(fixture.provider.id, signal())).keys).toHaveLength(3);
  });
});
