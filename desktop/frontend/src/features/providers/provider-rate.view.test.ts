import { createElement } from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import fixture from '../../../../contract/provider-account.json';
import workspace from '../../../../contract/provider-workspace.json';
import { checkedWorkspace } from './provider-features.model';
import { GroupRateObservation, KeyGroupName, ProviderKeyRates } from './provider-rate.view';
import type { KeyRateCache } from '../../desktop/contract.generated';

const key = checkedWorkspace(workspace).keys[0];
const rate: KeyRateCache = { multiplier: 0.01, group: 'friends', group_name: '群友', basis: 'authenticated_group', observed_at: '2026-10-20T12:00:00Z', attempted_at: '2026-10-20T12:00:00Z', failure: '' };

describe('Key billing rate display', () => {
  it('shows the effective rate and its basis without inventing a group', () => {
    const html = renderToStaticMarkup(createElement(GroupRateObservation, { value: fixture.key_billing_rate }));
    expect(html).toContain('当前计费倍率');
    expect(html).toContain('1.2×');
    expect(html).toContain('含个人覆盖及高峰规则');
    expect(html).not.toContain('分组  ');
  });

  it('shows every Key including disabled, free and unknown rates', () => {
    const html = renderToStaticMarkup(createElement(ProviderKeyRates, { keys: [
      { ...key, id: 'DS', rate, effective_multiplier: 0.005 },
      { ...key, id: '官key', rate: { ...rate, multiplier: 1, group_name: '官方' }, effective_multiplier: 0.5 },
      { ...key, id: '免费', enabled: false, state: 'disabled', rate: { ...rate, multiplier: 0, group_name: '免费组' }, effective_multiplier: 0 },
      { ...key, id: '未查询', rate: null, effective_multiplier: null },
    ] }));
    for (const text of ['DS', '官key', '免费', '未查询', '群友', '官方', '免费组', '已停用', '0.01×', '0.005×', '1×', '0.5×', '0×', '未知']) expect(html).toContain(text);
  });

  it('retains cached rates and refresh failures for each Key', () => {
    const html = renderToStaticMarkup(createElement(ProviderKeyRates, { keys: [
      { ...key, rate: { ...rate, failure: 'network' }, effective_multiplier: null },
    ] }));
    expect(html).toContain('群友'); expect(html).toContain('0.01×');
    expect(html).toContain('缓存于'); expect(html).toContain('刷新失败');
    expect(html).toContain('未知');
  });

  it('uses the group name, then group ID, and never invents a group', () => {
    const render = (value: KeyRateCache | null) => renderToStaticMarkup(createElement(KeyGroupName, { rate: value }));
    expect(render(rate)).toContain('群友');
    expect(render({ ...rate, group_name: '' })).toContain('friends');
    expect(render({ ...rate, group: '', group_name: '', basis: 'key_billing' })).toContain('未知');
    expect(render(null)).toContain('未知');
  });
});
