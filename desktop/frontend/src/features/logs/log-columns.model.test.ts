import { describe, expect, it } from 'vitest';
import { defaultColumns, moveColumn, normalizeColumns, outputTps } from './log-columns.model';
import { requestFixtures } from './log.mock';

describe('log column preferences', () => {
  it('recovers stale or corrupt preferences without losing a usable table', () => {
    expect(normalizeColumns(null)).toEqual(defaultColumns);
    expect(normalizeColumns([])).toEqual(defaultColumns);
    expect(normalizeColumns(['deleted-column'])).toEqual(defaultColumns);
    expect(normalizeColumns(['tps', 'deleted-column', 'model', 'tps', 2])).toEqual(['tps', 'model']);
  });
  it('keeps the requested order and ignores movement beyond either boundary', () => {
    expect(moveColumn(['time', 'tps', 'model'], 'tps', -1)).toEqual(['tps', 'time', 'model']);
    expect(moveColumn(['time', 'tps', 'model'], 'tps', 1)).toEqual(['time', 'model', 'tps']);
    expect(moveColumn(['time', 'model'], 'time', -1)).toEqual(['time', 'model']);
    expect(moveColumn(['time', 'model'], 'model', 1)).toEqual(['time', 'model']);
  });
});
describe('observed text TPS', () => {
  it('uses confirmed text tokens over the complete text generation interval, excluding wait/retry and reasoning', () => {
    const row = requestFixtures[1];
    expect(outputTps(row)).toBeCloseTo(326 / 1.2);
    expect(outputTps({ ...row, elapsed: 9999, output: 9000, metrics: { ...row.metrics!, reasoningTokens: 8000 } })).toBeCloseTo(326 / 1.2);
  });
  it('keeps incomplete, non-streaming, reasoning-only and invalid intervals unknown', () => {
    const row = requestFixtures[1];
    expect(outputTps(requestFixtures[0])).toBeNull();
    expect(outputTps(requestFixtures[3])).toBeNull();
    expect(outputTps(requestFixtures[4])).toBeNull();
    expect(outputTps({ ...row, stream: false })).toBeNull();
    for (const generationMs of [0, -1, null, NaN, Infinity]) {
      expect(outputTps({ ...row, metrics: { ...row.metrics!, generationMs } })).toBeNull();
    }
    expect(outputTps({ ...row, metrics: { ...row.metrics!, outputTextTokens: null } })).toBeNull();
    expect(outputTps({ ...row, metrics: { ...row.metrics!, textIntervalComplete: false } })).toBeNull();
  });
});
