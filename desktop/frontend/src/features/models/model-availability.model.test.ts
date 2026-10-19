import { describe, expect, it } from 'vitest';
import providers from '../../../../contract/providers.json';
import workspace from '../../../../contract/provider-workspace.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import { aggregateModels, summarize } from '../providers/provider-summary.model';
import { channelKeys } from '../providers/channel-availability.model';
import { modelAvailability, visibleCandidates, visibleKeys, visibleModels } from './model-availability.model';

const source = (id: string, enabled: boolean, state = 'ready', keyEnabled = true) => summarize({ ...providers.providers[0], id, enabled }, { ...workspace, keys: [{ ...workspace.keys[0], state, enabled: keyEnabled }], models: [{ ...mapping, candidate_count: 1 }] });

describe('model availability presentation order', () => {
  it('moves unavailable models behind usable and unknown ones, preserving original arrays', () => {
    const groups = [source('off', false), source('on', true), source('recovering', true, 'half-open')].flatMap((s, i) => aggregateModels([{ ...s, workspace: { ...s.workspace!, models: s.workspace!.models.map(m => ({ ...m, id: `${i}` })) } }]));
    const before = JSON.stringify(groups);
    const split = visibleModels(groups);
    expect(split.visible.map(g => g.alias)).toEqual(['1', '2']);
    expect(split.unavailable.map(g => g.alias)).toEqual(['0']);
    expect(JSON.stringify(groups)).toBe(before);
  });
  it('retains configured candidate order while showing usable sources first', () => {
    const summaries = [source('off', false), source('recovering', true, 'half-open'), source('on', true)].map((s, i) => ({ ...s, workspace: { ...s.workspace!, models: [{ ...mapping, candidate_index: i, candidate_count: 3 }] } }));
    const group = aggregateModels(summaries)[0];
    const before = JSON.stringify(group);
    const split = visibleCandidates(group.candidates);
    expect(split.visible.map(c => c.provider_id)).toEqual(['recovering', 'on']);
    expect(split.unavailable.map(c => c.provider_id)).toEqual(['off']);
    expect(JSON.stringify(group)).toBe(before);
  });
  it('never treats an incomplete snapshot as evidence of unavailability or deletion', () => {
    const group = aggregateModels([source('off', false)])[0];
    group.complete = false;
    expect(modelAvailability(group)).toBe('unknown');
    expect(visibleModels([group]).unavailable).toEqual([]);
  });
  it('restores enabled Keys to the visible group without losing scope or inventing anonymous access', () => {
    const keys = [{ ...workspace.keys[0], id: 'off', enabled: false, state: 'disabled' }, { ...workspace.keys[0], id: 'on' }];
    const scope = ['off', 'on'];
    const split = visibleKeys(channelKeys(scope, keys));
    expect(split.visible.map(k => k.id)).toEqual(['on']);
    expect(split.unavailable.map(k => k.id)).toEqual(['off']);
    keys[0].enabled = true; keys[0].state = 'ready';
    expect(visibleKeys(channelKeys(scope, keys)).visible.map(k => k.id)).toEqual(scope);
    expect(visibleKeys(channelKeys(['deleted'], keys)).visible).toEqual([]);
  });
});
