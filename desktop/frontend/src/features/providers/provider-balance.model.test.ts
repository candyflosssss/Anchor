import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/providers.json';
import account from '../../../../contract/provider-account.json';
import { ProviderBalanceCache } from './provider-balance.model';

const provider = fixture.providers[0];
const value = { observation: { ...account.summary, provider_id: provider.id, revision: provider.revision } };

describe('balance observations across provider toggles', () => {
  it('keeps the observation and query identity through repeated confirmed toggles', () => {
    const cache = new ProviderBalanceCache('test');
    cache.write(provider, value);
    const disabled = { ...provider, enabled: false, revision: provider.revision + 1 };
    cache.retainToggle(provider, disabled); cache.prune([disabled]);
    const enabled = { ...disabled, enabled: true, revision: disabled.revision + 1 };
    cache.retainToggle(disabled, enabled); cache.prune([enabled]);
    expect(cache.read(enabled)).toBe(value);
    expect(cache.queryRevision(enabled)).toBe(provider.revision);
    // The timestamp and observed revision remain truthful historical observations.
    expect(cache.read(enabled)?.observation?.revision).toBe(provider.revision);
  });

  it('also keeps the identity when the first read is still pending or queued', () => {
    const cache = new ProviderBalanceCache('test');
    const next = { ...provider, enabled: false, revision: provider.revision + 1 };
    cache.retainToggle(provider, next);
    expect(cache.read(next)).toBeUndefined();
    expect(cache.queryRevision(next)).toBe(provider.revision);
  });

  it.each([
    { base_urls: ['https://changed.example.com/v1'] }, { has_secret: false }, { protocol: 'anthropic' },
    { timeout_seconds: 30 }, { max_concurrent: 1 }, { cost_multiplier: 2 }, { homepage_url: '' },
    { id: 'other' }, { revision: provider.revision + 2 }, { enabled: true },
  ])('never carries an unproven or additional configuration change: %j', changes => {
    const cache = new ProviderBalanceCache('test');
    cache.write(provider, value);
    const next = { ...provider, enabled: false, revision: provider.revision + 1, ...changes };
    cache.retainToggle(provider, next); cache.prune([next]);
    expect(cache.read(next)).toBeUndefined();
    expect(cache.queryRevision(next)).toBe(next.revision);
  });

  it('invalidates after a later configuration change, deletion or instance switch', () => {
    const cache = new ProviderBalanceCache('test');
    cache.write(provider, value);
    const next = { ...provider, enabled: false, revision: provider.revision + 1 };
    cache.retainToggle(provider, next);
    const edited = { ...next, revision: next.revision + 1 };
    cache.prune([edited]);
    expect(cache.read(edited)).toBeUndefined();
    expect(cache.queryRevision(edited)).toBe(edited.revision);
    cache.write(next, value); cache.prune([]);
    expect(cache.read(next)).toBeUndefined();
    expect(new ProviderBalanceCache('other').read(next)).toBeUndefined();
  });
});
