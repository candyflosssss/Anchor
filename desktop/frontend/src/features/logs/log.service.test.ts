import { afterEach, expect, it, vi } from 'vitest';
import suggestionsFixture from '../../../../contract/log-suggestions.json';
import fixture from '../../../../contract/logs.json';
import type { Request } from '../../desktop/contract.generated';
import { logService, requestLog } from './log.service';
import { outputTps } from './log-columns.model';
afterEach(() => vi.unstubAllGlobals());
it('shares canonical log fields and preserves missing versus zero', () => {
  const failed = requestLog(fixture.page.requests[0] as Request);
  expect(failed).toMatchObject({ http: 200, code: 'invalid_response', input: null, stream: false });
  expect(failed.metrics?.timeToFirstTextMs).toBeNull();
  const success = requestLog(fixture.page.requests[1] as Request);
  expect(success.metrics?.cacheReadTokens).toBe(0);
  expect(success.reasoningEffort).toBe('high');
  expect(success.referenceCost?.usd).toBe('0.00123');
  expect(success.attempts[0].referenceUSD).toBe('0.00123');
  expect(failed.referenceCost).toBeUndefined();
  expect(outputTps(success)).toBeCloseTo(57.1429, 3);
  expect(outputTps(failed)).toBeNull();
});
it('fails without a host and rejects malformed rows before rendering', async () => {
  vi.stubGlobal('window', {});
  await expect(logService.list(fixture.query)).rejects.toMatchObject({ code: 'unsupported' });
  vi.stubGlobal('window', { go: { desktop: { Bridge: { Logs: async () => ({ contract: fixture.contract, data: { ...fixture.page, requests: [{ status: 'invalid' }] }, problem: null }) } } } });
  await expect(logService.list(fixture.query)).rejects.toMatchObject({ code: 'read_error' });
});

it('explains an HTML response while preserving the HTTP status and structural evidence', () => {
  const row = { ...fixture.page.requests[0], code: 'unexpected_html', evidence: { response_kind: 'html', endpoint_resolution: 'v1_fallback', endpoint_requests: '2' } } as Request;
  const result = requestLog(row);
  expect(result).toMatchObject({ http: 200, code: 'unexpected_html', evidence: row.evidence });
  expect(result.summary).toContain('HTML');
  expect(result.summary).toContain('/v1');
});

it('preserves authentication diagnostics even if an error response has HTML headers', () => {
  const row = { ...fixture.page.requests[0], code: 'authentication', evidence: { response_kind: 'html' } } as Request;
  expect(requestLog(row).summary).toBe('authentication');
});

it('prefers effective output timing and falls back to legacy text timing', () => {
  const row = fixture.page.requests[1] as Request;
  const legacy = structuredClone(row);
  Reflect.deleteProperty(legacy.metrics, 'time_to_first_output_ms');
  expect(requestLog(legacy).metrics?.timeToFirstOutputMs).toBe(row.metrics.time_to_first_text_ms);
  const tool = { ...row, metrics: { ...row.metrics, time_to_first_output_ms: 123, time_to_first_text_ms: null } };
  expect(requestLog(tool).metrics).toMatchObject({ timeToFirstOutputMs: 123, timeToFirstTextMs: null });
  expect(requestLog({ ...tool, metrics: { ...tool.metrics, time_to_first_output_ms: 0 } }).metrics?.timeToFirstOutputMs).toBe(0);
});

it('preserves legacy interval metrics without using them for total TPS', () => {
  const row = structuredClone(fixture.page.requests[1]) as Request;
  row.metrics.generation_ms = 1000;
  row.metrics.output_text_tokens = 60;
  row.metrics.text_interval_complete = true;
  for (const omitted of [true, false]) {
    const legacy = structuredClone(row);
    if (omitted) {
      for (const field of ['output_generation_ms', 'non_reasoning_output_tokens', 'output_interval_complete']) Reflect.deleteProperty(legacy.metrics, field);
    } else {
      Object.assign(legacy.metrics, { output_generation_ms: null, non_reasoning_output_tokens: null, output_interval_complete: false });
    }
    expect(requestLog(legacy).metrics).toMatchObject({ outputGenerationMs: 1000, nonReasoningOutputTokens: 60 });
    expect(outputTps(requestLog(legacy))).toBeCloseTo(120 / 2.1);
    legacy.metrics.output_text_tokens = null;
    expect(outputTps(requestLog(legacy))).toBeCloseTo(120 / 2.1);
  }
  // Missing non-reasoning usage does not invalidate a confirmed total output.
  row.metrics.non_reasoning_output_tokens = null;
  expect(outputTps(requestLog(row))).toBeCloseTo(120 / 2.1);
});


it('shares the suggestions contract and forwards only a scoped query', async () => {
  const suggestions = vi.fn(async () => ({ contract: suggestionsFixture.contract, data: suggestionsFixture.suggestions, problem: null }));
  vi.stubGlobal('window', { go: { desktop: { Bridge: { LogSuggestions: suggestions } } } });
  expect(await logService.suggestions(suggestionsFixture.query)).toEqual(suggestionsFixture.suggestions);
  expect(suggestions).toHaveBeenCalledWith(suggestionsFixture.query);
});
it('rejects missing and malformed suggestion responses', async () => {
  vi.stubGlobal('window', {});
  await expect(logService.suggestions(suggestionsFixture.query)).rejects.toMatchObject({ code: 'unsupported' });
  for (const data of [null, { items: null }, { items: [{ value: '', count: 1 }], has_more: false }, { items: [{ value: 'x', count: -1 }], has_more: false }]) {
    vi.stubGlobal('window', { go: { desktop: { Bridge: { LogSuggestions: async () => ({ contract: fixture.contract, data, problem: null }) } } } });
    await expect(logService.suggestions(suggestionsFixture.query)).rejects.toMatchObject({ code: 'read_error' });
  }
});
