import { describe, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/provider-workspace.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import { executeMappingBatch, mappingBatchAction, type MappingBatchOperation, type MappingBatchReport } from './provider-mapping-batch.model';
import type { Workspace } from './provider-features.model';
import { ProviderProblem } from './provider.model';
import { MockProviderTransport } from './provider.mock';

function workspace(): Workspace {
  return { ...structuredClone(fixture), models: [
    { ...structuredClone(mapping), id: 'first', upstream: 'vendor/one', tools: false, service_tier: 'priority', key_ids: ['default'], revision: 4 },
    { ...structuredClone(mapping), id: 'second', upstream: 'vendor/two', streaming: false, strategy: 'priority', revision: 9 },
    { ...structuredClone(mapping), id: 'third', upstream: 'vendor/three', revision: 2 },
  ] };
}
function setup(operation: MappingBatchOperation = 'edit') {
  const w = workspace(), transport = new MockProviderTransport(), abort = new AbortController();
  const action = vi.spyOn(transport, 'action').mockResolvedValue(w);
  const progress = vi.fn<(report: MappingBatchReport) => void>(), saved = vi.fn();
  const options = { workspace: w, ids: ['first', 'second', 'third'], operation, patch: { tools: true }, transport, signal: abort.signal, stopping: () => false, progress, saved };
  return { w, action, abort, options, saved, progress };
}

describe('mapping batch actions', () => {
  it('preserves per-mapping fields and revisions instead of copying the first row', () => {
    const w = workspace();
    expect(mappingBatchAction(w, 'first', 'edit', { streaming: false })).toMatchObject({
      action: 'model-put', target: 'first', model: 'vendor/one', model_revision: 4, expected_revision: 3,
      tools: false, streaming: false, service_tier: 'priority', strategy: 'round-robin', key_ids: ['default'], multiplier: 0,
    });
    expect(mappingBatchAction(w, 'second', 'edit', { tools: false })).toMatchObject({ tools: false, streaming: false, strategy: 'priority', key_ids: [], model: 'vendor/two', model_revision: 9 });
  });
  it('honors explicit false, empty tier and all-enabled Key bindings', () => {
    expect(mappingBatchAction(workspace(), 'first', 'edit', { tools: false, streaming: false, service_tier: '', key_ids: [] })).toMatchObject({ tools: false, streaming: false, service_tier: '', key_ids: [] });
  });
  it('keeps the saved upstream and respects disabled/missing bound Keys without fallback', () => {
    const w = workspace();
    w.keys.push({ ...w.keys[0], id: 'other' }); w.keys[0].enabled = false;
    expect(mappingBatchAction(w, 'first', 'test')).toBeNull();
    expect(mappingBatchAction(w, 'second', 'test')).toMatchObject({ action: 'test', target: 'other', model: 'vendor/two', approved: true });
    w.keys = [];
    expect(mappingBatchAction(w, 'first', 'test')).toBeNull();
    expect(mappingBatchAction(w, 'second', 'test')).toMatchObject({ target: 'anonymous' });
  });
  it('deletes only this provider mapping with approval and a frozen model revision', () => {
    expect(mappingBatchAction(workspace(), 'first', 'delete')).toMatchObject({ provider_id: '摸鱼AI', action: 'model-delete', target: 'first', model_revision: 4, approved: true });
    expect(() => mappingBatchAction(workspace(), 'missing', 'delete')).toThrow(ProviderProblem);
  });
});

describe('serial batch execution', () => {
  it('deduplicates rows and never adopts newer provider or model revisions mid-batch', async () => {
    const { w, action, options } = setup();
    action.mockResolvedValueOnce({ ...w, revision: 8, models: w.models.map(model => ({ ...model, revision: 100 })) });
    const report = await executeMappingBatch({ ...options, ids: ['first', 'second', 'second'] });
    expect(action).toHaveBeenCalledTimes(2);
    expect(action.mock.calls[1][0]).toMatchObject({ expected_revision: 3, model_revision: 9, model: 'vendor/two' });
    expect(report.status).toBe('completed'); expect(report.items.map(item => item.status)).toEqual(['success', 'success']);
  });
  it.each(['conflict', 'unknown', 'invalid'] as const)('stops on %s, preserves partial success and never retries', async code => {
    const { action, options, saved } = setup('delete');
    action.mockRejectedValueOnce(new ProviderProblem(code));
    const report = await executeMappingBatch(options);
    expect(action).toHaveBeenCalledTimes(1); expect(saved).not.toHaveBeenCalled();
    expect(report).toMatchObject({ status: 'stopped', needsRead: true });
    expect(report.items.map(item => item.status)).toEqual(['failed', 'pending', 'pending']);
  });
  it('keeps successful writes when a later write fails', async () => {
    const { w, action, options, saved } = setup('delete');
    action.mockResolvedValueOnce(w).mockRejectedValueOnce(new ProviderProblem('unknown'));
    const report = await executeMappingBatch(options);
    expect(action).toHaveBeenCalledTimes(2); expect(saved).toHaveBeenCalledTimes(1);
    expect(report.items.map(item => item.status)).toEqual(['success', 'failed', 'pending']);
  });
  it('waits for the current request before stopping the queue', async () => {
    const { w, action, options } = setup();
    let stopping = false;
    action.mockImplementationOnce(async () => { stopping = true; return w; });
    const report = await executeMappingBatch({ ...options, stopping: () => stopping });
    expect(action).toHaveBeenCalledTimes(1); expect(report.status).toBe('stopped');
    expect(report.items.map(item => item.status)).toEqual(['success', 'pending', 'pending']);
    expect(report.needsRead).toBe(false);
  });
  it('does not overlap requests and publishes independent progress snapshots', async () => {
    const { w, action, options, progress } = setup();
    let finish!: (value: Workspace) => void;
    action.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const pending = executeMappingBatch(options);
    expect(action).toHaveBeenCalledTimes(1);
    const first = progress.mock.calls[0][0];
    expect(first.items.map(item => item.status)).toEqual(['pending', 'pending', 'pending']);
    finish(w); await pending;
    expect(action).toHaveBeenCalledTimes(3);
    expect(first.items.map(item => item.status)).toEqual(['pending', 'pending', 'pending']);
  });
  it('continues after upstream probe failure, skips unusable bindings and retains each result', async () => {
    const { w, action, options } = setup('test');
    w.models[0].key_ids = ['missing'];
    action.mockImplementation(async input => ({ ...w, probe: { revision: 3, key_id: input.target, model: input.model, at: 1, http_status: input.model === 'vendor/two' ? 429 : 200, elapsed_ms: 10, failure: input.model === 'vendor/two' ? 'rate_limited' : '' } }));
    const report = await executeMappingBatch(options);
    expect(action).toHaveBeenCalledTimes(2);
    expect(report.items.map(item => item.status)).toEqual(['skipped', 'failed', 'success']);
    expect(report.items[1].probe?.model).toBe('vendor/two');
    expect(report.status).toBe('completed'); expect(report.needsRead).toBe(false);
  });
  it('does not accept a stale or missing probe as success', async () => {
    const { options, action } = setup('test');
    const report = await executeMappingBatch(options);
    expect(action).toHaveBeenCalledTimes(1); expect(report.needsRead).toBe(true);
    expect(report.items[0].status).toBe('failed');
  });
  it('rejects a workspace returned for another provider before publishing it or continuing', async () => {
    const { w, options, action, saved } = setup();
    action.mockResolvedValueOnce({ ...w, provider_id: 'other' });
    const report = await executeMappingBatch(options);
    expect(saved).not.toHaveBeenCalled(); expect(action).toHaveBeenCalledTimes(1);
    expect(report).toMatchObject({ status: 'stopped', needsRead: true });
  });
  it('rejects late results after disposal without publishing a workspace or starting another item', async () => {
    const { w, action, options, abort, saved } = setup();
    action.mockImplementationOnce(async () => { abort.abort(); return w; });
    const report = await executeMappingBatch(options);
    expect(saved).not.toHaveBeenCalled(); expect(action).toHaveBeenCalledTimes(1);
    expect(report).toMatchObject({ status: 'stopped', needsRead: true });
  });
});
