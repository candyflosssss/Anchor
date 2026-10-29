import { describe, expect, it } from 'vitest';
import probeFixture from '../../../../contract/provider-probe.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import fixture from '../../../../contract/provider-workspace.json';
import { actionFor, catalogImportPlan, checkedWorkspace, keyNeedsRecovery, type Workspace } from './provider-features.model';
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

describe('key model protocol settings', () => {
  it('validates and sanitizes protocols belonging to each key', () => {
    const w: Workspace = structuredClone(fixture);
    w.keys[0].model_protocols = [Object.assign({ model: 'actual/model', protocol: 'anthropic' }, { secret: 'hidden' })];
    const result = checkedWorkspace(w);
    expect(result.keys[0].model_protocols).toEqual([{ model: 'actual/model', protocol: 'anthropic' }]);
    expect(() => checkedWorkspace({ ...w, keys: [{ ...w.keys[0], protocol: 'invalid' }] })).toThrow();
    expect(() => checkedWorkspace({ ...w, keys: [{ ...w.keys[0], model_protocols: [{ model: 'actual/model', protocol: 'invalid' }] }] })).toThrow();
  });
  it('keeps model overrides scoped to the selected key and clears them on inherit', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    let p = (await transport.list(signal))[0];
    let w = await transport.action(actionFor(p, 'key-put', { target: 'second', secret: 'synthetic', protocol: 'anthropic' }), signal);
    expect(w.keys.find(k => k.id === 'second')?.protocol).toBe('anthropic');
    p = { ...p, revision: w.revision };
    w = await transport.action(actionFor(p, 'key-model-protocol', { target: 'second', model: 'actual/model', protocol: 'chat-completions' }), signal);
    expect(w.keys.find(k => k.id === 'second')?.model_protocols).toEqual([{ model: 'actual/model', protocol: 'chat-completions' }]);
    expect(w.keys.find(k => k.id === 'default')?.model_protocols).toEqual([]);
    const catalog = await transport.discover({ provider_id: p.id, expected_revision: w.revision, key_id: 'second' }, signal);
    expect(catalog.key_id).toBe('second');
    p = { ...p, revision: w.revision };
    w = await transport.action(actionFor(p, 'key-model-protocol', { target: 'second', model: 'actual/model', protocol: 'inherit' }), signal);
    expect(w.keys.find(k => k.id === 'second')?.model_protocols).toEqual([]);
    expect(w.keys.find(k => k.id === 'second')?.protocol).toBe('anthropic');
    await expect(transport.discover({ provider_id: p.id, expected_revision: w.revision, key_id: 'missing' }, signal)).rejects.toMatchObject({ code: 'invalid' });
  });
});


describe('shared public model mappings', () => {
  it('validates shared routing fields from the Go/TS fixture', () => {
    const w = { ...structuredClone(fixture), models: [mapping] };
    expect(checkedWorkspace(w).models).toEqual([mapping]);
    for (const multiplier of [-1, 0, NaN, Infinity, 1001]) {
      expect(() => checkedWorkspace({ ...w, models: [{ ...mapping, multiplier }] })).toThrow();
    }
    expect(() => checkedWorkspace({ ...w, models: [{ ...mapping, strategy: 'invalid' }] })).toThrow();
  });
  it('attaches, edits and removes one provider while preserving the shared model', async () => {
    const transport = new MockProviderTransport();
    const signal = new AbortController().signal;
    const [a, b] = await transport.list(signal);
    await transport.action(actionFor(a, 'model-put', { target: 'shared', model: 'up-a', tools: true, streaming: true, multiplier: 0.5 }), signal);
    const joined = await transport.action(actionFor(b, 'model-put', { target: 'shared', model: 'up-b', multiplier: 2 }), signal);
    expect(joined.models.find(m => m.id === 'shared')).toMatchObject({ revision: 2, candidate_count: 2, tools: true, streaming: true, multiplier: 2 });
    await expect(transport.action(actionFor(b, 'model-put', { target: 'shared', model: 'overwrite' }), signal)).rejects.toMatchObject({ code: 'conflict' });
    await expect(transport.action(actionFor(a, 'model-put', { target: 'shared', model: 'stale', model_revision: 1 }), signal)).rejects.toMatchObject({ code: 'conflict' });
    await transport.action(actionFor(a, 'model-put', { target: 'shared', model: 'new-a', model_revision: 2, multiplier: 1, strategy: 'priority', tools: true, streaming: true }), signal);
    const other = await transport.workspace(b.id, signal);
    expect(other.models.find(m => m.id === 'shared')).toMatchObject({ revision: 3, strategy: 'priority', upstream: 'up-b', multiplier: 2 });
    await transport.action(actionFor(a, 'model-delete', { target: 'shared', model_revision: 3, approved: true }), signal);
    expect((await transport.workspace(b.id, signal)).models.find(m => m.id === 'shared')).toMatchObject({ revision: 4, candidate_count: 1, upstream: 'up-b' });
  });
});


describe('catalog import planning', () => {
  it('deduplicates and skips existing upstreams, conflicting aliases and invalid public IDs', () => {
    const w = { ...structuredClone(fixture), models: [{ ...mapping, id: 'taken', upstream: 'mapped-upstream' }] };
    expect(catalogImportPlan(['mapped-upstream', 'valid', 'valid', 'taken', 'vendor/model', '中文', '-bad', 'a'.repeat(129)], w)).toEqual({
      ready: ['valid', '中文'], mapped: 1, manual: ['taken', 'vendor/model', '-bad', 'a'.repeat(129)],
    });
  });
});

it('preserves the shared HTML failure in persisted workspace results', () => {
  const value = { ...structuredClone(fixture), probe: probeFixture.html_failure.result };
  expect(checkedWorkspace(value).probe?.failure).toBe('unexpected_html');
});


describe('recover all provider Keys', () => {
  it('offers recovery for failures, error scores and scheduling restrictions', () => {
    for (const state of ['invalid', 'cooling', 'half-open', 'degraded']) expect(keyNeedsRecovery({ state, failures: 0 })).toBe(true);
    expect(keyNeedsRecovery({ state: 'disabled', failures: 1 })).toBe(true);
    expect(keyNeedsRecovery({ state: 'ready', failures: 0, error_score: 25 })).toBe(true);
    for (const state of ['ready', 'disabled']) expect(keyNeedsRecovery({ state, failures: 0 })).toBe(false);
  });

  it('resets all health without changing enabled flags, revisions or other providers', async () => {
    const transport = new MockProviderTransport();
    const signal = new AbortController().signal;
    const rows = await transport.list(signal);
    const p = rows.find(p => p.id === '备用供应商')!;
    const other = rows.find(row => row.id !== p.id)!;
    const otherBefore = await transport.workspace(other.id, signal);
    let w = await transport.action(actionFor(p, 'key-put', { target: 'disabled', secret: 'synthetic', enabled: false }), signal);
    const current = { ...p, revision: w.revision };
    await expect(transport.action(actionFor(current, 'key-reset-all'), signal)).rejects.toMatchObject({ code: 'invalid' });
    await expect(transport.action(actionFor(p, 'key-reset-all', { approved: true }), signal)).rejects.toMatchObject({ code: 'conflict' });
    await expect(transport.action(actionFor(current, 'key-reset-all', { approved: true, target: 'primary' }), signal)).rejects.toMatchObject({ code: 'invalid' });
    const enabled = w.keys.map(k => k.enabled);
    w = await transport.action(actionFor(current, 'key-reset-all', { approved: true }), signal);
    expect(w.revision).toBe(current.revision);
    expect(w.keys.map(k => k.enabled)).toEqual(enabled);
    expect(w.keys.every(k => !keyNeedsRecovery(k) && k.retry_at === 0 && k.test === null)).toBe(true);
    expect(w.keys.find(k => k.id === 'disabled')?.state).toBe('disabled');
    expect(await transport.workspace(other.id, signal)).toEqual(otherBefore);
  });
});
