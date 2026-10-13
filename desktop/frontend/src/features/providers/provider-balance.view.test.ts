import { describe, expect, it } from 'vitest';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import fixture from '../../../../contract/provider-account.json';
import { ProviderBalanceCell } from './provider-balance.view';
import { checkedAccountObservation } from './provider-account.model';

const query = { provider_id: 'demo', expected_revision: 3, operation: 'balance-summary', key_id: '', remote_key_id: 0 };
describe('provider billing balance display', () => {
  it('shows an authorized NewAPI summary as an account wallet', () => {
    const observation = checkedAccountObservation(fixture.newapi_account_summary, query);
    const html = renderToStaticMarkup(createElement(ProviderBalanceCell, { value: { observation } }));
    expect(html).toContain('345 quota（原始额度）');
    expect(html).toContain('账户余额');
    expect(html).not.toContain('Key 额度');
    expect(html).not.toContain('站点余额');
  });
  it('shows the billing amount and site scope for a Key-only NewAPI query', () => {
    const observation = checkedAccountObservation(fixture.billing_summary, query);
    const html = renderToStaticMarkup(createElement(ProviderBalanceCell, { value: { observation } }));
    expect(html).toContain('19.12 USD');
    expect(html).toContain('站点余额');
    expect(html).toContain('可能是账户或所选 Key');
    expect(html).toContain('Key：default');
    expect(html).not.toContain('需额外授权');
  });
  it('preserves unknown currency instead of displaying dollars', () => {
    const observation = checkedAccountObservation({ ...fixture.billing_summary, allowance: {
      ...fixture.billing_summary.allowance, remaining: { value: '19.12', unit: 'site_unit' },
    } }, query);
    const html = renderToStaticMarkup(createElement(ProviderBalanceCell, { value: { observation } }));
    expect(html).toContain('币种未确认');
    expect(html).not.toContain('USD');
  });
  it('continues to label a fallback result as Key quota', () => {
    const observation = checkedAccountObservation(fixture.key_summary, query);
    const html = renderToStaticMarkup(createElement(ProviderBalanceCell, { value: { observation } }));
    expect(html).toContain('Key 额度');
    expect(html).not.toContain('站点余额');
  });
});
