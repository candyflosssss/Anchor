import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/model-settings.json';
import type { ModelCatalog, ModelConfiguration } from '../../desktop/contract.generated';
import { checkedConfiguration, checkedModelSettings, createModelSettingsService, importConfiguration } from './model-settings.model';
import { DesktopProblem } from '../../desktop/transport';
import { CONTRACT } from '../../desktop/contract.generated';
const config = () => checkedConfiguration(structuredClone(fixture.configuration));
const catalog = (): ModelCatalog => ({ source: 'models.dev', fetched_at: 1, expires_at: 2, version: 'a'.repeat(64), stale: false, failure: '', providers: [], total: 1, truncated: false, models: [{ provider_id: 'lab', id: 'm', canonical_model_id: '', name: 'M', source_updated_at: '', limits: { context: 500000, input: 400000, output: 64000 }, capabilities: { input_modalities: ['text'], output_modalities: ['text'], vision: false, attachment: null, tool_call: false, reasoning: true, structured_output: null, temperature: false }, reasoning_options: [{ type: 'effort', values: ['low'], min: null, max: null }], pricing: null }] });
describe('model mapping settings', () => {
  it('checks identity, limits and native reasoning range', () => {
    expect(checkedModelSettings(fixture, 'mapped').configuration.limits.context).toBe(272000);
    expect(() => checkedModelSettings(fixture, 'other')).toThrow();
    const c = config(); c.default_reasoning!.value = 'medium'; expect(() => checkedConfiguration(c)).toThrow();
    c.default_reasoning = null; c.default_output_tokens = 32001; expect(() => checkedConfiguration(c)).toThrow();
    c.default_output_tokens = null; c.limits.context = NaN; expect(() => checkedConfiguration(c)).toThrow();
  });
  it('preserves manual overrides and defaults when filling empty fields', () => {
    const c = config(), imported = importConfiguration(c, catalog(), false);
    expect(imported.limits).toEqual({ context: 272000, input: 400000, output: 32000 });
    expect(imported.capabilities.vision).toBe(true); expect(imported.capabilities.tool_call).toBe(true);
    expect(imported.default_reasoning).toEqual(c.default_reasoning); expect(imported.supported_reasoning).toEqual(c.supported_reasoning);
    expect(c.limits.input).toBeNull();
  });
  it('replaces metadata only after explicit selection and clears incompatible defaults', () => {
    const imported = importConfiguration(config(), catalog(), true);
    expect(imported.limits.context).toBe(500000); expect(imported.capabilities.vision).toBe(false);
    expect(imported.supported_reasoning[0].values).toEqual(['low']); expect(imported.default_reasoning).toBeNull();
  });
  it('keeps toggle, null effort and budget reasoning separate', () => {
    const c: ModelConfiguration = config();
    c.supported_reasoning = [{ type: 'budget_tokens', values: [], min: 1024, max: 4096 }]; c.default_reasoning = { type: 'budget_tokens', value: null, enabled: null, budget_tokens: 2048 };
    expect(checkedConfiguration(c).default_reasoning?.budget_tokens).toBe(2048);
    c.default_reasoning.budget_tokens = 500; expect(() => checkedConfiguration(c)).toThrow();
    c.supported_reasoning = [{ type: 'toggle', values: [], min: null, max: null }]; c.default_reasoning = { type: 'toggle', value: null, enabled: false, budget_tokens: null }; expect(checkedConfiguration(c).default_reasoning?.enabled).toBe(false);
    c.supported_reasoning = [{ type: 'effort', values: [null, 'none'], min: null, max: null }]; c.default_reasoning = { type: 'effort', value: null, enabled: null, budget_tokens: null }; expect(checkedConfiguration(c).default_reasoning?.value).toBeNull();
  });
  it('does not replay uncertain saves and prevents cancelled reads leaking state', async () => {
    const save = vi.fn(async () => { throw Error('private'); }); const read = vi.fn(async () => ({ contract: CONTRACT, data: fixture, problem: null }));
    const service = createModelSettingsService(() => ({ ModelSettings: read, SaveModelSettings: save }));
    await expect(service.save({ model_id: 'mapped', expected_revision: 1, display_name: '', configuration: config(), reference_action: 'keep', reference_provider_id: '', reference_model_id: '' }, new AbortController().signal)).rejects.toMatchObject({ code: 'unknown' });
    expect(save).toHaveBeenCalledTimes(1);
    const request = new AbortController(); const result = service.read('mapped', request.signal); request.abort(); await expect(result).rejects.toThrow();
  });
  it('rejects a response from another model or wrong revision after writing', async () => {
    const service = createModelSettingsService(() => ({ ModelSettings: async () => ({ contract: CONTRACT, data: { ...fixture, model_id: 'other' }, problem: null }), SaveModelSettings: async () => ({ contract: CONTRACT, data: { ...fixture, revision: 4 }, problem: null }) }));
    await expect(service.read('mapped', new AbortController().signal)).rejects.toBeInstanceOf(DesktopProblem);
    await expect(service.save({ model_id: 'mapped', expected_revision: 1, display_name: '', configuration: config(), reference_action: 'keep', reference_provider_id: '', reference_model_id: '' }, new AbortController().signal)).rejects.toMatchObject({ code: 'unknown' });
  });
});
