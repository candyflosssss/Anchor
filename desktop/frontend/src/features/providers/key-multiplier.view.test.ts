import { describe, expect, it } from 'vitest';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import fixture from '../../../../contract/provider-workspace.json';
import { checkedWorkspace } from './provider-features.model';
import { KeyMultiplierView } from './key-multiplier.view';
import type { KeyRateCache, KeyStatus } from '../../desktop/contract.generated';

const rate: KeyRateCache = { multiplier: 2, group: 'premium', group_name: 'Premium', basis: 'authenticated_group', observed_at: '2026-10-15T00:00:00Z', attempted_at: '2026-10-15T00:00:00Z', failure: '' };
const key = checkedWorkspace(fixture).keys[0];
const render = (value?: KeyStatus) => renderToStaticMarkup(createElement(KeyMultiplierView, { value }));
describe('core-derived per-Key multipliers', () => {
  it('displays cached group and effective multiplier separately', () => {
    const html = render({ ...key, rate, effective_multiplier: 1 });
    expect(html).toContain('2×'); expect(html).toContain('1×'); expect(html).toContain('缓存于');
  });
  it('keeps zero valid and failed refresh visible', () => {
    const html = render({ ...key, rate: { ...rate, multiplier: 0, failure: 'network' }, effective_multiplier: 0 });
    expect(html).toContain('0×'); expect(html).toContain('沿用上次成功缓存');
  });
  it('does not invent a 1x group or relative cost for unknown Keys', () => {
    expect(render()).toContain('未知'); expect(render()).not.toContain('1×');
    expect(render({ ...key, rate: { ...rate, multiplier: null, observed_at: '', failure: 'network' }, effective_multiplier: null })).toContain('不按 1× 处理');
  });
  it('rejects invalid cache and computed rates at the transport boundary', () => {
    for (const value of [-1, NaN, Infinity]) {
      expect(() => checkedWorkspace({ ...fixture, keys: [{ ...key, rate: { ...rate, multiplier: value } }] })).toThrow();
      expect(() => checkedWorkspace({ ...fixture, keys: [{ ...key, effective_multiplier: value }] })).toThrow();
    }
  });
});
