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
  status: () => undefined, busy: false, error: '', needsRead: false,
  change: async () => {}, move: async () => {}, refresh: async () => {}, retry: async () => {}, undo: async () => {}, discard: () => {}, routeID: c => JSON.stringify([c.provider_id, c.model]),
};
const makeSummary = (enabled: boolean, state = 'ready', keyEnabled = true, candidateCount = 1) => summarize({ ...providers.providers[0], enabled }, { ...workspace, keys: [{ ...workspace.keys[0], state, enabled: keyEnabled }], models: [{ ...mapping, candidate_count: candidateCount }] });
const render = (summaries: ReturnType<typeof summarize>[]) => renderToStaticMarkup(createElement(ModelsView, { summaries, loading: false, disabled: false, rates, routing, onOpen: () => {}, onShowProviders: () => {} }));

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
  it('shows currently available without asserting upstream connectivity', () => {
    const html = render([makeSummary(true)]);
    expect(html).toContain('1 / 1 个渠道当前可用');
    expect(html).toContain('不代表上游实时连通');
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
  it('shows usable candidates before closed unavailable sources without changing routing indices', () => {
    const off = makeSummary(false), on = makeSummary(true);
    off.provider.id = 'off'; on.provider.id = 'on';
    off.workspace!.models[0] = { ...mapping, candidate_count: 2, candidate_index: 0 };
    on.workspace!.models[0] = { ...mapping, candidate_count: 2, candidate_index: 1 };
    const html = render([off, on]);
    expect(html).toContain('调整来源顺序');
    const section = html.match(/<details[^>]*aria-label="当前不可用的来源"[^>]*>/)?.[0];
    expect(section).toBeDefined(); expect(section).not.toContain('open');
    expect(html.indexOf('第 2 位')).toBeLessThan(html.indexOf('当前不可用的来源'));
    expect(html.indexOf('当前不可用的来源')).toBeLessThan(html.indexOf('第 1 位'));
    expect(html).not.toContain('aria-label="下移 on');
  });

});
