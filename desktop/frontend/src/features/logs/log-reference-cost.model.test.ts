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

it('keeps snapshot factors and category amounts in copy output and attempt mapping', () => {
  const row = requestLog(fixture.page.requests[1] as Request);
  expect(row.attempts[0].costAdjustment).toEqual(row.referenceCost?.adjustment);
  const summary = safeSummary(row);
  for (const text of ['原始参考费用: $0.00123', '供应商倍率: ×0.5', '分组倍率: ×0.8', '最终参考费用: $0.000492']) expect(summary).toContain(text);
});
