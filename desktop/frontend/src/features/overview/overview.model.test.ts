import { describe, expect, it } from 'vitest';
import providers from '../../../../contract/providers.json';
import workspace from '../../../../contract/provider-workspace.json';
import { summarize } from '../providers/provider-summary.model';
import { overviewMetrics } from './overview.model';

const provider = providers.providers[0];
const ready = () => summarize(provider, structuredClone(workspace));
const metric = (label: string, ...args: Parameters<typeof overviewMetrics>) => overviewMetrics(...args).find(m => m.label === label)!;

describe('overview observation states', () => {
  it('does not show measured zeros while loading or after a list failure', () => {
    expect(overviewMetrics([], true).every(m => m.value === '…')).toBe(true);
    expect(overviewMetrics([], false, true).every(m => m.value === '—' && m.hint === '读取失败')).toBe(true);
  });
  it('keeps known provider counts but marks all unavailable dependent metrics', () => {
    const summaries = [summarize(provider, null)];
    expect(metric('供应商', summaries, false).value).toBe('1 / 1');
    expect(overviewMetrics(summaries, false).slice(1).every(m => m.value === '—' && m.hint === '读取失败')).toBe(true);
  });
  it('marks partially observed totals rather than presenting them as complete', () => {
    const summaries = [ready(), summarize({ ...provider, id: 'unavailable' }, null)];
    for (const m of overviewMetrics(summaries, false).slice(1)) {
      expect(m.hint).toContain('统计不完整');
      if (m.value !== '—') expect(m.value).toMatch(/\+$/);
    }
  });
  it('distinguishes missing cost estimates from a measured zero', () => {
    const s = ready();
    s.requests = 1; s.workspace!.usage = [{ provider_id: provider.id, model: 'unknown-price', requests: 1, failures: 0, elapsed_ms: 1, known: 0, input_tokens: 0, output_tokens: 0, cache_read: 0, cache_write: 0 }];
    s.workspace!.estimates = [{ model: 'unknown-price', usd: null }]; s.usd = 0; s.usdPartial = true;
    expect(metric('估算费用', [s], false)).toMatchObject({ value: '—', hint: '暂无费用估算' });
    s.workspace!.estimates[0].usd = 0; s.usdPartial = false;
    expect(metric('估算费用', [s], false)).toMatchObject({ value: '$0.0000', hint: 'USD' });
  });
  it('allows true empty and partially priced results without manufacturing unknown values', () => {
    expect(metric('请求', [], false).value).toBe('0');
    expect(metric('估算费用', [], false).value).toBe('$0.0000');
    const s = ready(); s.usd = 0.0123; s.usdPartial = true; s.workspace!.estimates = [{ model: 'known-price', usd: 0.0123 }];
    expect(metric('估算费用', [s], false)).toMatchObject({ value: '$0.0123+', hint: 'USD · 估算不完整' });
  });
});
