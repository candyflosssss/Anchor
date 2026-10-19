import { afterEach, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/log-stats.json';
import { chartPaths, checkedStats, formatStat, statsQuery, validInterval } from './log-stats.model';
import { initialFilters } from './log.model';
import { logService } from './log.service';
afterEach(() => vi.unstubAllGlobals());
it('shares canonical bucket, summary, zero and null observations', () => {
  const value = checkedStats(structuredClone(fixture.stats));
  expect(value.summary).toMatchObject({ requests: 3, total_tokens: 600, usage_samples: 2, tps: 50, cache_hit_rate: 20 });
  expect(value.buckets[1].values.tps).toBeNull();
  expect(value.buckets[2].values.cache_hit_rate).toBe(0);
  expect(formatStat(null)).toBe('—');
  expect(formatStat(0)).toBe('0');
});
it('mirrors all request filters but not pagination and snapshots', () => {
  const query = statsQuery({ ...initialFilters, window: '10080', status: 'failed', source: 'probe', provider: 'test', model: 'm', key: 'k', http: '200', code: 'invalid_response', stage: 'response_validate', query: ' req_ ' }, 3600000, 1800000000000);
  expect(query).toMatchObject({ interval_ms: 3600000, filter: { kind: 'requests', since_ms: 1800000000000 - 7 * 86400000, before_ms: 1800000000000, status: 'failed', source: 'probe', provider: 'test', model: 'm', key: 'k', http: 200, code: 'invalid_response', stage: 'response_validate', search: 'req_', cursor: 0, until: 0 } });
  expect(validInterval('129600', 10000)).toBe(false);
  expect(validInterval('129600', 86400000)).toBe(true);
  expect(validInterval('129600', 0)).toBe(true);
});
it('returns missing chart ticks to zero at their original positions', () => {
  expect(chartPaths([0, 5, null, 10], 10)).toEqual(['M0.00,180.00 L333.33,90.00 L666.67,180.00 L1000.00,0.00']);
  expect(chartPaths([5, null, null, 10], 10)).toEqual(['M0.00,90.00 L333.33,180.00 L666.67,180.00 L1000.00,0.00']);
  expect(chartPaths([null, 5, null], 10, 1000, 180, [100, 400, 900])).toEqual(['M100.00,180.00 L400.00,90.00 L900.00,180.00']);
  expect(chartPaths([null, null], 1)).toEqual(['M0.00,180.00 L1000.00,180.00']);
  expect(chartPaths([null], 1)).toEqual(['M500.00,180.00']);
  expect(chartPaths([], 1)).toEqual([]);
  expect(chartPaths([0], 1)).toEqual(['M500.00,180.00']);
  expect(chartPaths([5, 10], 10, 1000, 180, [250, 750])).toEqual(['M250.00,90.00 L750.00,0.00']);
});
it('rejects malformed, non-finite, out-of-range and missing observations', () => {
  for (const change of [
    (v: typeof fixture.stats) => { v.summary.tps = Infinity; },
    (v: typeof fixture.stats) => { v.summary.error_rate = 101; },
    (v: typeof fixture.stats) => { v.buckets[0].at_ms++; },
    (v: typeof fixture.stats) => { v.buckets.pop(); },
    (v: typeof fixture.stats) => { Reflect.deleteProperty(v.summary, 'total_tokens'); },
  ]) { const value = structuredClone(fixture.stats); change(value); expect(() => checkedStats(value)).toThrow(); }
});
it('reads through the controlled bridge and never falls back on unavailable hosts', async () => {
  vi.stubGlobal('window', {});
  await expect(logService.stats(fixture.query)).rejects.toMatchObject({ code: 'unsupported' });
  vi.stubGlobal('window', { go: { desktop: { Bridge: { LogStats: async () => ({ contract: fixture.contract, data: fixture.stats, problem: null }) } } } });
  expect(await logService.stats(fixture.query)).toEqual(fixture.stats);
});
