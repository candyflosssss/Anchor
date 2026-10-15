import { describe, expect, it } from 'vitest';
import { defaultColumns, moveColumn, normalizeColumns, outputTps } from './log-columns.model';
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
describe('observed output TPS', () => {
  it('uses confirmed non-reasoning output tokens over the complete output interval, excluding wait/retry and reasoning', () => {
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
    for (const outputGenerationMs of [0, -1, null, NaN, Infinity]) {
      expect(outputTps({ ...row, metrics: { ...row.metrics!, outputGenerationMs } })).toBeNull();
    }
    expect(outputTps({ ...row, metrics: { ...row.metrics!, nonReasoningOutputTokens: null } })).toBeNull();
    expect(outputTps({ ...row, metrics: { ...row.metrics!, outputIntervalComplete: false } })).toBeNull();
  });
});

it('does not substitute total output or body-text metrics for missing output measurements', () => {
  const row = requestFixtures[1];
  for (const nonReasoningOutputTokens of [null, undefined, 0, -1, NaN, Infinity]) {
    expect(outputTps({ ...row, metrics: { ...row.metrics!, nonReasoningOutputTokens } })).toBeNull();
  }
  expect(outputTps({ ...row, metrics: { ...row.metrics!, outputGenerationMs: undefined } })).toBeNull();
});
