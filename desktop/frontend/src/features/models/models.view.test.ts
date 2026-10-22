import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import providers from '../../../../contract/providers.json';
import workspace from '../../../../contract/provider-workspace.json';
import mapping from '../../../../contract/provider-model-mapping.json';
import { summarize } from '../providers/provider-summary.model';
import { ModelsView } from './models.view';
import type { MappingRatesController } from './use-mapping-rates';
import type { ModelRoutingController } from './use-model-routing';

const rates: MappingRatesController = { values: {}, keys: {}, errors: {}, busy: false, pending: '', refresh: async () => {} };
const routing: ModelRoutingController = {
  draft: g => ({ strategy: g.strategy, order: g.candidates.map(c => ({ provider_id: c.provider_id, model: c.upstream })) }),
  status: () => undefined, busy: false, error: '', needsRead: false, sorting: null, canProbe: true,
  sortByPrice: async () => {}, sortByLatency: async () => {}, stopSorting: () => {},
  change: async () => {}, move: async () => {}, refresh: async () => {}, retry: async () => {}, undo: async () => {}, discard: () => {}, routeID: c => JSON.stringify([c.provider_id, c.model]),
};
const makeSummary = (enabled: boolean, state = 'ready', keyEnabled = true, candidateCount = 1) => summarize({ ...providers.providers[0], enabled }, { ...workspace, keys: [{ ...workspace.keys[0], state, enabled: keyEnabled }], models: [{ ...mapping, candidate_count: candidateCount }] });
const render = (summaries: ReturnType<typeof summarize>[], rateState = rates) => renderToStaticMarkup(createElement(ModelsView, { summaries, loading: false, disabled: false, rates: rateState, routing, onOpen: () => {}, onShowProviders: () => {} }));

describe('model channel availability presentation', () => {
  it('marks zero valid channels and retains disabled Key details', () => {
    const html = render([makeSummary(true, 'disabled', false)]);
    expect(html).toContain('availability-unavailable');
    expect(html).toContain('无有效渠道');
    expect(html).toContain('0 / 1 个渠道当前可用');
    expect(html).toContain('供应商已启用');
    expect(html).toContain('Key 已停用');
    expect(html).not.toContain('匿名来源');
  });
  it('makes provider and Key switches distinct', () => {
    const html = render([makeSummary(false)]);
    expect(html).toContain('供应商已停用');
    expect(html).toContain('Key 已启用');
    expect(html).toContain('无有效渠道');
  });
  it('does not label incomplete or recovering routes as definitely unavailable', () => {
    for (const summary of [makeSummary(true, 'disabled', false, 2), makeSummary(true, 'half-open')]) {
      const html = render([summary]);
      expect(html).toContain('可用性待确认');
      expect(html).not.toContain('无有效渠道');
    }
  });
  it('shows the current availability and channel count', () => {
    const html = render([makeSummary(true)]);
    expect(html).toContain('1 / 1 个渠道当前可用');
    expect(html).not.toContain('无有效渠道');
  });
  it('does not mistake failed reads for an empty configuration', () => {
    const html = render([summarize(providers.providers[0], null)]);
    expect(html).toContain('无法确认模型映射');
    expect(html).toContain('部分供应商状态读取失败');
    expect(html).not.toContain('还没有模型映射');
  });
  it('puts unavailable model cards in a closed trailing section', () => {
    const off = makeSummary(false), on = makeSummary(true);
    off.workspace!.models[0].id = 'a-off'; on.workspace!.models[0].id = 'z-on';
    const html = render([off, on]);
    const section = html.match(/<details[^>]*aria-label="当前不可用的模型"[^>]*>/)?.[0];
    expect(section).toBeDefined(); expect(section).not.toContain('open');
    expect(html.indexOf('模型 z-on')).toBeLessThan(html.indexOf('当前不可用的模型'));
    expect(html.indexOf('当前不可用的模型')).toBeLessThan(html.indexOf('模型 a-off'));
  });
  it('keeps every channel visible in saved priority order', () => {
    const off = makeSummary(false), on = makeSummary(true);
    off.provider.id = 'off'; on.provider.id = 'on';
    off.workspace!.models[0] = { ...mapping, candidate_count: 2, candidate_index: 0 };
    on.workspace!.models[0] = { ...mapping, candidate_count: 2, candidate_index: 1 };
    const html = render([off, on]);
    expect(html).toContain('按倍率');
    expect(html).toContain('低延迟优先');
    expect(html).not.toContain('aria-label="当前不可用的来源"');
    expect(html.indexOf('第 1 位')).toBeLessThan(html.indexOf('第 2 位'));
    expect(html).toContain('aria-label="下移 off');
  });

  it('shows one card per scoped Key with its own multiplier and scheduling state', () => {
    const summary = makeSummary(true);
    const base = summary.workspace!.keys[0];
    summary.workspace!.keys = [
      { ...base, id: 'cheap', effective_multiplier: 0.2 },
      { ...base, id: 'expensive', effective_multiplier: 3 },
      { ...base, id: 'cooling', state: 'cooling', effective_multiplier: 0.01 },
      { ...base, id: 'unrelated', effective_multiplier: 0.001 },
    ];
    summary.workspace!.models[0] = { ...summary.workspace!.models[0], multiplier: 1, key_ids: ['cheap', 'expensive', 'cooling', 'missing'] };
    const html = render([summary]);
    expect(html.match(/class="mapping-key-card /g)).toHaveLength(4);
    expect(html).toContain('0.2×</strong>');
    expect(html).toContain('3×</strong>');
    expect(html).toContain('冷却中');
    expect(html).toContain('Key 不存在');
    expect(html).not.toContain('来源 Key unrelated');
    expect(html).not.toContain('class="mapping-keys"');
    expect(html).not.toContain('当前不可用的 Key');
  });
  it('uses refreshed billing per Key without refreshing its scheduling state', () => {
    const summary = makeSummary(true, 'cooling');
    const id = JSON.stringify([summary.provider.id, summary.workspace!.keys[0].id]);
    const html = render([summary], { ...rates, keys: { [id]: { ...summary.workspace!.keys[0], state: 'ready', effective_multiplier: 0.4 } } });
    expect(html).toContain('冷却中');
    expect(html).toContain('相对倍率 0.4×');
    expect(html).toContain('aria-label="排序倍率 未知"');
  });
  it('labels genuinely anonymous routes without inventing a Key', () => {
    const summary = makeSummary(true);
    summary.workspace!.keys = [];
    summary.workspace!.models[0].key_ids = [];
    const html = render([summary]);
    expect(html.match(/class="mapping-key-card /g)).toHaveLength(1);
    expect(html).toContain('匿名来源');
    expect(html).not.toContain('Key 已启用');
  });

});
