import { createElement } from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import fixture from '../../../../contract/provider-account.json';
import { GroupRateObservation } from './provider-rate.view';

describe('Key billing rate display', () => {
  it('shows the effective rate and its basis without inventing a group', () => {
    const html = renderToStaticMarkup(createElement(GroupRateObservation, { value: fixture.key_billing_rate }));
    expect(html).toContain('当前计费倍率');
    expect(html).toContain('1.2×');
    expect(html).toContain('含个人覆盖及高峰规则');
    expect(html).not.toContain('分组  ');
  });
});
