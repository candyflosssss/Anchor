import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-workspace.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import providers from '../../../../contract/providers.json';
import { channelAvailability, channelKeys, keyAvailability } from './channel-availability.model';
import { aggregateModels, summarize } from './provider-summary.model';
import type { KeyStatus } from '../../desktop/contract.generated';
const key = (id: string, state = 'ready', enabled = true): KeyStatus => ({ ...fixture.keys[0], id, state, enabled });

describe('mapping channel availability', () => {
  it('a provider switch takes precedence over an enabled Key', () => {
    expect(channelAvailability(false, [], [key('a')])).toMatchObject({ state: 'unavailable', reason: expect.stringContaining('供应商已停用') });
    expect(channelAvailability(true, [], [key('a')]).state).toBe('available');
  });
  it('keeps disabled Keys visible and never mistakes all-disabled Keys for anonymous', () => {
    const keys = [key('a', 'disabled', false)];
    expect(channelKeys([], keys)).toEqual([{ id: 'a', key: keys[0], anonymous: false }]);
    expect(channelAvailability(true, [], keys)).toMatchObject({ state: 'unavailable', reason: expect.stringContaining('已停用') });
  });
  it('uses the mapping scope, not unrelated healthy Keys on the provider', () => {
    const keys = [key('bad', 'invalid'), key('ok')];
    expect(channelAvailability(true, ['bad'], keys).state).toBe('unavailable');
    expect(channelAvailability(true, [], keys).state).toBe('available');
    expect(channelAvailability(true, ['missing'], keys).state).toBe('unavailable');
  });
  it('distinguishes cooling, invalid, and ambiguous half-open status', () => {
    for (const state of ['cooling', 'invalid', 'disabled']) expect(channelAvailability(true, [], [key('a', state)]).state).toBe('unavailable');
    for (const state of ['half-open', 'future-state']) expect(channelAvailability(true, [], [key('a', state)]).state).toBe('unknown');
    expect(channelAvailability(true, [], [key('a', 'half-open'), key('b')]).state).toBe('available');
    expect(keyAvailability({ id: 'a', key: key('a', 'ready', false), anonymous: false }).state).toBe('unavailable');
  });
  it('allows anonymous only when the core would synthesize an anonymous Key', () => {
    expect(channelAvailability(true, [], []).state).toBe('available');
    expect(channelAvailability(true, ['anonymous'], []).state).toBe('available');
    expect(channelAvailability(true, ['deleted'], []).state).toBe('unavailable');
    expect(channelAvailability(true, ['anonymous'], [key('a')]).state).toBe('unavailable');
    expect(channelKeys(['anonymous'], [key('anonymous')])[0].anonymous).toBe(false);
  });
  it('derives per-mapping availability instead of reusing provider-wide health', () => {
    const summary = summarize({ ...providers.providers[0], enabled: true }, { ...fixture, keys: [key('bad', 'invalid'), key('ok')], models: [{ ...mapping, key_ids: ['bad'] }] });
    const [group] = aggregateModels([summary]);
    expect(group.candidates[0]).toMatchObject({ provider_enabled: true, availability: { state: 'unavailable' } });
  });
});
