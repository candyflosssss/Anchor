import { expect, it } from 'vitest';
import { referenceUSD } from './log-reference-cost.model';
import { safeSummary } from './log.model';
import { requestLog } from './log.service';
import fixture from '../../../../contract/logs.json';
import type { Request } from '../../desktop/contract.generated';

it('keeps exact decimal amounts including zero and very small values', () => {
  expect(referenceUSD('0.000000000000000001')).toBe('$0.000000000000000001');
  expect(referenceUSD('1234.560000000000000000')).toBe('$1,234.56');
  expect(referenceUSD('0')).toBe('$0');
  for (const value of [null, undefined, '-1', 'NaN', '1e-3']) expect(referenceUSD(value)).toBe('—');
});
it('copies the captured reference cost and accepts historical logs without it', () => {
  expect(safeSummary(requestLog(fixture.page.requests[1] as Request))).toContain('参考费用: $0.00123');
  expect(safeSummary(requestLog(fixture.page.requests[0] as Request))).toContain('参考费用: —');
});
