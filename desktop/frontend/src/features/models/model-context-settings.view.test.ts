import { describe, expect, it } from 'vitest';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import fixture from '../../../../contract/model-settings.json';
import model from '../../../../contract/context-pricing.json';
import { checkedModelSettings } from './model-settings.model';
import { ContextPriceNotice, ContextPriceTable } from './model-context-settings.view';

describe('context price explanations', () => {
  it('shows the actual 272k whole-request bands instead of hiding tier JSON', () => {
    const html = renderToStaticMarkup(createElement(ContextPriceTable, { pricing: model.pricing }));
    expect(html).toContain('≤ 272,000'); expect(html).toContain('&gt; 272,000');
    for (const rate of ['2', '10', '0.1', '2.5', '4', '15', '0.2', '5']) expect(html).toContain(`<td>${rate}</td>`);
    expect(html).toContain('整次请求单价');
    expect(html).not.toContain('<pre>');
  });
  it('separates capacity from a saved long window and requires an acknowledgement', () => {
    const state = checkedModelSettings(fixture, 'mapped');
    state.context_policy = model.context_policy; state.configuration.limits.context = 500000;
    const html = renderToStaticMarkup(createElement(ContextPriceNotice, { state, acknowledged: false, acknowledge: () => {}, change: () => {} }));
    expect(html).toContain('1,050,000'); expect(html).toContain('500,000'); expect(html).toContain('272,000');
    expect(html).toContain('我已了解跨档费用'); expect(html).toContain('不是只对超出部分加价');
    expect(html).toContain('不能据此确认 Fast / Ultrafast');
    expect(html).toContain('不是上游费用硬限额');
  });
});
