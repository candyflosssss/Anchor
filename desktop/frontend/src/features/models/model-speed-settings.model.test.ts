import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-workspace.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import { checkedWorkspace } from '../providers/provider-features.model';
import { speedSettingsAction, speedSettingsError } from './model-speed-settings.model';
const workspace = () => checkedWorkspace({ ...structuredClone(fixture), models: [{ ...mapping, key_ids: ['default'], tools: false, streaming: false }] });
describe('mapping settings speed transaction', () => {
  it('preserves the source channel, shared strategy, capabilities and both revision guards', () => {
    const w = workspace(), a = speedSettingsAction(w, mapping.id, '', ['fast', 'ultrafast']);
    expect(a).toMatchObject({ action: 'model-put', provider_id: w.provider_id, expected_revision: w.revision,
      target: mapping.id, model: mapping.upstream, model_revision: mapping.revision,
      strategy: mapping.strategy, key_ids: ['default'], tools: false, streaming: false,
      service_tier: '', speed_variants: ['fast', 'ultrafast'] });
    a.key_ids!.push('other'); expect(w.models[0].key_ids).toEqual(['default']);
  });
  it.each(['', 'priority', 'ultrafast'])('explicitly writes or clears the current policy: %s', tier => {
    expect(speedSettingsAction(workspace(), mapping.id, tier, []).service_tier).toBe(tier);
  });
  it('blocks a missing or deleted source and invalid tiers', () => {
    expect(() => speedSettingsAction(workspace(), 'missing', '', [])).toThrow();
    expect(() => speedSettingsAction({ ...workspace(), deleted: true }, mapping.id, '', [])).toThrow();
    expect(() => speedSettingsAction(workspace(), mapping.id, 'fast', [])).toThrow();
  });
  it('requires an ordinary unfixed base and valid, non-duplicate suffixes', () => {
    const w = workspace();
    for (const [id, tier, variants] of [[mapping.id, 'priority', ['fast']], ['m(fast)', '', ['ultrafast']], [mapping.id, '', ['fast', 'fast']], [mapping.id, '', ['turbo']], ['a'.repeat(125), '', ['ultrafast']]] as [string, string, string[]][]) {
      expect(speedSettingsError(w, id, tier, variants)).not.toBe('');
    }
    expect(speedSettingsError(w, mapping.id, '', ['fast', 'ultrafast'])).toBe('');
  });
  it('blocks known alias collisions without overwriting them', () => {
    const w = workspace(); w.models.push({ ...w.models[0], id: `${mapping.id}(fast)` });
    expect(speedSettingsError(w, mapping.id, '', ['fast'])).toContain('已有同名速度分身');
    expect(() => speedSettingsAction(w, mapping.id, '', ['fast'])).toThrow();
  });
});
