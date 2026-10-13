import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/model-catalog.json';
import { CONTRACT, type CatalogEstimateInput, type ModelCatalogQuery } from '../../desktop/contract.generated';
import { checkedCatalogEstimate, checkedModelCatalog, createModelCatalogService } from './model-catalog.model';

const query: ModelCatalogQuery = { provider_id: 'lab', model_id: 'vision-reasoner', search: '', limit: 0, refresh: false };
const input: CatalogEstimateInput = { query, usage: fixture.estimate.usage };
const api = () => ({ ModelCatalog: vi.fn(async () => ({ contract: CONTRACT, data: checkedModelCatalog(fixture.catalog), problem: null })), EstimateModelCost: vi.fn(async () => ({ contract: CONTRACT, data: checkedCatalogEstimate(fixture.estimate), problem: null })) });
describe('models.dev reference methods', () => {
  it('preserves exact effort/null, budgets, decimal prices and context tiers', async () => {
    const host = api(), service = createModelCatalogService(() => host);
    expect(await service.query(query)).toEqual(fixture.catalog);
    expect(await service.estimate(input)).toEqual(fixture.estimate);
    const model = checkedModelCatalog(fixture.catalog).models[0];
    expect(model.reasoning_options[0].values[0]).toBeNull();
    expect(model.reasoning_options[1].min).toBe(-1);
    expect(model.pricing?.tiers[0].above_tokens).toBe(250000);
    expect(host.ModelCatalog).toHaveBeenCalledOnce();
  });
  it('preserves unknown values and stale provenance without replacing them with false or zero', () => {
    const stale = structuredClone(fixture.catalog);
    stale.stale = true; stale.failure = 'refresh_failed';
    const data = checkedModelCatalog(stale);
    data.models[0].capabilities.tool_call = null; data.models[0].limits.context = null;
    data.models[0].pricing!.rates.cache_read = null;
    expect(checkedModelCatalog(data).models[0].capabilities.tool_call).toBeNull();
    expect(checkedModelCatalog(data).models[0].pricing!.rates.cache_read).toBeNull();
    const unknown = { ...fixture.estimate, usd: null, reason: 'missing_cache_usage', usage: { ...fixture.estimate.usage, cache_read_tokens: null } };
    expect(checkedCatalogEstimate(unknown).usd).toBeNull();
  });
  it('rejects malformed or inconsistent metadata and money', () => {
    expect(() => checkedModelCatalog({ ...fixture.catalog, version: 'bad' })).toThrow('read_error');
    expect(() => checkedModelCatalog({ ...fixture.catalog, models: [{ ...fixture.catalog.models[0], limits: { context: 0, input: null, output: null } }] })).toThrow('read_error');
    expect(() => checkedCatalogEstimate({ ...fixture.estimate, usd: 'NaN' })).toThrow('read_error');
    expect(() => checkedCatalogEstimate({ ...fixture.estimate, reason: 'missing_usage' })).toThrow('read_error');
    expect(() => checkedCatalogEstimate({ ...fixture.estimate, usage: { ...fixture.estimate.usage, cache_read_tokens: 2000 } })).toThrow('read_error');
    const data = checkedModelCatalog({ ...fixture.catalog, secret: 'discard', models: [{ ...fixture.catalog.models[0], secret: 'discard' }] });
    expect(JSON.stringify(data)).not.toContain('secret');
  });
  it('rejects a model or usage returned for a different request', async () => {
    const service = createModelCatalogService(api);
    await expect(service.query({ ...query, provider_id: 'other' })).rejects.toMatchObject({ code: 'read_error' });
    await expect(service.estimate({ ...input, usage: { ...input.usage, input_tokens: 2000 } })).rejects.toMatchObject({ code: 'read_error' });
  });
  it('discards a late result after cancellation and never replays read failures', async () => {
    const host = api(), controller = new AbortController(), service = createModelCatalogService(() => host);
    const pending = service.query(query, controller.signal); controller.abort();
    await expect(pending).rejects.toThrow();
    await expect(service.query(query, controller.signal)).rejects.toThrow();
    expect(host.ModelCatalog).toHaveBeenCalledOnce();
    host.ModelCatalog.mockRejectedValueOnce(new Error('synthetic-private-detail'));
    await expect(service.query(query)).rejects.toMatchObject({ code: 'offline', message: 'offline' });
    expect(host.ModelCatalog).toHaveBeenCalledTimes(2);
  });
});
