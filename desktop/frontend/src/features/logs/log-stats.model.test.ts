import { afterEach, expect, it, vi } from 'vitest';
import fixture from '../../../../contract/log-stats.json';
import { chartInterval, chartPaths, checkedStats, formatStat, statsQuery, validInterval } from './log-stats.model';
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
it('uses readable server bucket sizes for automatic bars without overriding manual granularity', () => {
  expect(chartInterval('60', 0, 'errors', false)).toBe(300000);
  expect(chartInterval('15', 0, 'errors', false)).toBe(30000);
  expect(chartInterval('15', 0, 'errors', true)).toBe(60000);
  expect(chartInterval('129600', 0, 'errors', false)).toBe(86400000);
  expect(chartInterval('60', 10000, 'errors', true)).toBe(10000);
  expect(chartInterval('129600', 10000, 'errors', true)).toBe(86400000);
  expect(chartInterval('60', 0, 'tokens', false)).toBe(0);
});
// Read the actual SVG Bézier segments and evaluate them independently of PCHIP.
function curveSegments(values: (number | null)[], positions?: number[]) {
  const path = chartPaths(values, 100, 1000, 180, positions)[0];
  const coordinates = [...path.matchAll(/(-?[\d.]+),(-?[\d.]+)/g)].map(match => [Number(match[1]), Number(match[2])]);
  return Array.from({ length: (coordinates.length - 1) / 3 }, (_, i) => coordinates.slice(i * 3, i * 3 + 4));
}
it('passes through every sample and stays within adjacent values on uneven time intervals', () => {
  const values = [20, 60, 60, 15, 0, 0, 45, 25];
  const positions = [5, 50, 120, 300, 450, 700, 950, 995];
  const segments = curveSegments(values, positions);
  expect(segments).toHaveLength(values.length - 1);
  segments.forEach((points, i) => {
    const y0 = 180 - values[i] * 1.8, y1 = 180 - values[i + 1] * 1.8;
    expect(points[0]).toEqual([positions[i], y0]);
    expect(points[3]).toEqual([positions[i + 1], y1]);
    for (let sample = 0; sample <= 100; sample++) {
      const t = sample / 100;
      const y = (1 - t) ** 3 * points[0][1] + 3 * (1 - t) ** 2 * t * points[1][1] + 3 * (1 - t) * t ** 2 * points[2][1] + t ** 3 * points[3][1];
      expect(y).toBeGreaterThanOrEqual(Math.min(y0, y1) - .01);
      expect(y).toBeLessThanOrEqual(Math.max(y0, y1) + .01);
    }
  });
  for (let i = 1; i < segments.length; i++) {
    const left = segments[i - 1], right = segments[i];
    const incoming = (left[3][1] - left[2][1]) / (left[3][0] - left[2][0]);
    const outgoing = (right[1][1] - right[0][1]) / (right[1][0] - right[0][0]);
    expect(incoming).toBeCloseTo(outgoing, 2);
  }
});
it('keeps missing samples at zero and handles empty, single and two-point series', () => {
  const segments = curveSegments([null, 50, null, null], [100, 400, 700, 900]);
  expect(segments[0][0]).toEqual([100, 180]);
  expect(segments.map(points => points[3])).toEqual([[400, 90], [700, 180], [900, 180]]);
  expect(segments[2].every(point => point[1] === 180)).toBe(true);
  expect(chartPaths([], 1)).toEqual([]);
  expect(chartPaths([null], 1)).toEqual(['M500.00,180.00']);
  expect(chartPaths([0], 1)).toEqual(['M500.00,180.00']);
  const [points] = curveSegments([0, 100]);
  expect(points[0]).toEqual([0, 180]);
  expect(points[3]).toEqual([1000, 0]);
  expect(points[1][1]).toBe(120);
  expect(points[2][1]).toBe(60);
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
