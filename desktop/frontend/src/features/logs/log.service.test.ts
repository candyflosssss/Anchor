import { afterEach, expect, it, vi } from 'vitest';
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
  expect(outputTps(success)).toBeCloseTo(66.6667, 3);
  expect(outputTps(failed)).toBeNull();
});
it('fails without a host and rejects malformed rows before rendering', async () => {
  vi.stubGlobal('window', {});
  await expect(logService.list(fixture.query)).rejects.toMatchObject({ code: 'unsupported' });
  vi.stubGlobal('window', { go: { desktop: { Bridge: { Logs: async () => ({ contract: fixture.contract, data: { ...fixture.page, requests: [{ status: 'invalid' }] }, problem: null }) } } } });
  await expect(logService.list(fixture.query)).rejects.toMatchObject({ code: 'read_error' });
});
