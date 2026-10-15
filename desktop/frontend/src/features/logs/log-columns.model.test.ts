import { describe, expect, it } from 'vitest';
import { cacheHitRate, defaultColumns, moveColumn, normalizeColumns, outputTps } from './log-columns.model';
import { requestFixtures } from './log.mock';

describe('log column preferences', () => {
  it('recovers stale or corrupt preferences without losing a usable table', () => {
    expect(normalizeColumns(null)).toEqual(defaultColumns);
    expect(normalizeColumns([])).toEqual(defaultColumns);
    expect(normalizeColumns(['deleted-column'])).toEqual(defaultColumns);
    expect(normalizeColumns(['tps', 'deleted-column', 'model', 'timing', 2])).toEqual(['timing', 'model']);
  });
  it('migrates legacy fields into blocks without duplicates and preserves first occurrence order', () => {
    expect(normalizeColumns(['ttft', 'elapsed', 'model', 'upstreamModel', 'input', 'output', 'cacheReadTokens', 'reasoningTokens', 'key', 'provider', 'http', 'delivery', 'errorCode', 'stage', 'stream', 'protocol']))
      .toEqual(['timing', 'model', 'tokens', 'provider', 'status', 'diagnostics', 'protocol']);
    expect(normalizeColumns(['toString', '__proto__', 'constructor'])).toEqual(defaultColumns);
    expect(normalizeColumns(['tokens'])).toEqual(['tokens']);
  });
  it('keeps the requested order and ignores movement beyond either boundary', () => {
    expect(moveColumn(['time', 'timing', 'model'], 'timing', -1)).toEqual(['timing', 'time', 'model']);
    expect(moveColumn(['time', 'timing', 'model'], 'timing', 1)).toEqual(['time', 'model', 'timing']);
    expect(moveColumn(['time', 'model'], 'time', -1)).toEqual(['time', 'model']);
    expect(moveColumn(['time', 'model'], 'model', 1)).toEqual(['time', 'model']);
  });
});
describe('end-to-end total output TPS', () => {
  const row = { ...requestFixtures[1], output: 134, elapsed: 7847 };
  it('includes reasoning tokens once and uses total request time, including the initial wait', () => {
    expect(outputTps(row)).toBeCloseTo(17.08, 2);
    expect(outputTps({ ...row, metrics: { ...row.metrics!, reasoningTokens: 51,
      nonReasoningOutputTokens: 83, outputGenerationMs: 224 } })).toBeCloseTo(17.08, 2);
    expect(outputTps({ ...row, elapsed: 1000 })).toBe(134);
  });
  it.each([
    [134, 7847, 17.08], [153, 6183, 24.75], [191, 8006, 23.86],
    [673, 18270, 36.84], [223, 11465, 19.45],
  ])('calculates %i total output tokens over %i ms', (output, elapsed, expected) => {
    expect(outputTps({ ...row, output, elapsed })).toBeCloseTo(expected, 2);
  });
  it('supports non-streaming, reasoning-only and old logs without interval metrics', () => {
    expect(outputTps({ ...row, stream: false, metrics: undefined })).toBeCloseTo(17.08, 2);
    expect(outputTps({ ...row, stream: undefined, metrics: undefined })).toBeCloseTo(17.08, 2);
    expect(outputTps(requestFixtures[4])).toBeCloseTo(16 / 1.482);
    expect(outputTps({ ...row, generationStatus: 'incomplete' })).toBeCloseTo(17.08, 2);
    expect(outputTps({ ...row, metrics: { ...row.metrics!, outputIntervalComplete: false,
      outputGenerationMs: null, nonReasoningOutputTokens: null, reasoningTokens: null } })).toBeCloseTo(17.08, 2);
  });
  it('uses confirmed root totals over the full retry interval, not a last-attempt subtotal', () => {
    expect(outputTps({ ...row, output: 200, elapsed: 10000, attemptCount: 2 })).toBe(20);
    expect(outputTps({ ...row, output: null, attemptCount: 2 })).toBeNull();
  });
  it('keeps unfinished, failed and invalid measurements unknown but preserves confirmed zero', () => {
    for (const status of ['pending', 'failed', 'cancelled', 'interrupted'] as const) {
      expect(outputTps({ ...row, status })).toBeNull();
    }
    for (const elapsed of [undefined, null, 0, -1, NaN, Infinity]) {
      expect(outputTps({ ...row, elapsed: elapsed as number | null })).toBeNull();
    }
    for (const output of [undefined, null, -1, NaN, Infinity]) {
      expect(outputTps({ ...row, output: output as number | null })).toBeNull();
    }
    expect(outputTps({ ...row, output: 0 })).toBe(0);
  });
});

describe('cache hit rate', () => {
  const row = { ...requestFixtures[1], attemptCount: 1, input: 1200,
    metrics: { ...requestFixtures[1].metrics!, cacheReadTokens: 400 } };
  it('uses normalized input tokens as the denominator, without adding cached tokens twice', () => {
    expect(cacheHitRate(row)).toBeCloseTo(33.3333);
    expect(cacheHitRate({ ...row, metrics: { ...row.metrics, cacheReadTokens: 0 } })).toBe(0);
    expect(cacheHitRate({ ...row, metrics: { ...row.metrics, cacheReadTokens: 1200 } })).toBe(100);
  });
  it('leaves missing, invalid, zero-denominator and mixed-attempt values unknown', () => {
    for (const input of [null, 0, -1, NaN, Infinity]) expect(cacheHitRate({ ...row, input })).toBeNull();
    for (const cacheReadTokens of [null, -1, NaN, Infinity, 1201]) {
      expect(cacheHitRate({ ...row, metrics: { ...row.metrics, cacheReadTokens } })).toBeNull();
    }
    expect(cacheHitRate({ ...row, metrics: undefined })).toBeNull();
    for (const attemptCount of [0, 2, 65]) expect(cacheHitRate({ ...row, attemptCount })).toBeNull();
    expect(cacheHitRate({ ...row, attemptCount: undefined, attempts: [] })).toBeNull();
  });
});
