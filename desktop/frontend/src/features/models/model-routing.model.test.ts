import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/model-routing.json';
import workspace from '../../../../contract/provider-workspace.json';
import { checkedRouting, moveRoute } from './model-routing.model';
import { aggregateModels, summarize } from '../providers/provider-summary.model';
import providers from '../../../../contract/providers.json';
import { MockProviderTransport } from '../providers/provider.mock';
import { actionFor } from '../providers/provider-features.model';
import { DesktopProviderTransport } from '../../desktop/transport';
import type { DesktopBridge } from '../../desktop/contract.generated';

describe('persistent model routing contract', () => {
  it('checks the shared result and strips unexpected properties', () => {
    expect(checkedRouting({ ...fixture.success, extra: 'hidden' } as typeof fixture.success, fixture.input)).toEqual(fixture.success);
    expect(() => checkedRouting({ ...fixture.success, revision: 3 }, fixture.input)).toThrow('unknown');
    expect(() => checkedRouting({ ...fixture.success, order: [...fixture.success.order].reverse() }, fixture.input)).toThrow('unknown');
    expect(moveRoute(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
  });
  it('aggregates actual saved candidate order and blocks mixed revisions', () => {
    const mappings = fixture.success.order.map((ref, i) => summarize({ ...providers.providers[0], id: ref.provider_id }, { ...workspace, models: [{ id: 'public', display_name: 'public', upstream: ref.model, tools: true, streaming: true, revision: 2, candidate_count: 2, candidate_index: i, multiplier: 2, key_ids: ['default'], strategy: 'lowest-price' }] }));
    const [group] = aggregateModels(mappings.reverse());
    expect(group.complete).toBe(true); expect(group.candidates.map(c => c.provider_id)).toEqual(['b', 'a']); expect(group.candidates[0].key_ids).toEqual(['default']);
    mappings[0].workspace!.models[0].revision = 3;
    expect(aggregateModels(mappings)[0].complete).toBe(false);
  });
  it('retains imported Key scope and preserves it across atomic prototype routing writes', async () => {
    const transport = new MockProviderTransport(); const signal = new AbortController().signal;
    const p = (await transport.list(signal))[0];
    const w = await transport.action(actionFor(p, 'model-put', { target: 'scoped', model: 'up', key_ids: ['default'], multiplier: .5, tools: true, streaming: true }), signal);
    const m = w.models.find(m => m.id === 'scoped')!;
    await transport.routing({ model_id: m.id, expected_revision: m.revision, strategy: 'cache-priority', order: [{ provider_id: p.id, model: m.upstream }] }, signal);
    const scoped = (await transport.workspace(p.id, signal)).models.find(m => m.id === 'scoped')!;
    expect(scoped.key_ids).toEqual(['default']); expect(scoped.multiplier).toBe(.5); expect(scoped.strategy).toBe('cache-priority'); expect(scoped.tools).toBe(true);
  });
  it('does not retry a routing write with an unknown result', async () => {
    let calls = 0;
    const bridge = { ConfigureModelRouting: async () => { calls++; throw Error('private-details'); } } as unknown as DesktopBridge;
    await expect(new DesktopProviderTransport(() => bridge).routing(fixture.input, new AbortController().signal)).rejects.toMatchObject({ code: 'unknown' });
    expect(calls).toBe(1);
  });
});
