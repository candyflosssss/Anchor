import { expect, it } from 'vitest';
import fixture from '../../../../contract/logs.json';
import type { Request } from '../../desktop/contract.generated';
import { requestLog } from './log.service';
import { safeSummary, serviceTierLabel } from './log.model';

it('maps only reported tiers and distinguishes missing, auto and Standard', () => {
  expect(serviceTierLabel('priority')).toBe('Fast');
  expect(serviceTierLabel('fast')).toBe('Fast');
  expect(serviceTierLabel('ultrafast')).toBe('Ultrafast');
  expect(serviceTierLabel('default')).toBe('Standard');
  expect(serviceTierLabel('auto')).toBe('Auto（自动）');
  expect(serviceTierLabel('unspecified', true)).toBe('未指定（由上游决定）');
  for (const value of [undefined, null, '', 'unknown', 'SECRET', false, 42]) {
    expect(serviceTierLabel(value)).toBe('未知 / 未返回');
  }
});

it('preserves requested versus returned tiers through mapping and copy', () => {
  const row = requestLog({ ...fixture.page.requests[0], evidence: {
    requested_service_tier: 'priority', upstream_service_tier: 'default',
  } } as Request);
  expect(row.evidence.requested_service_tier).toBe('priority');
  expect(safeSummary(row)).toContain('请求档位: Fast');
  expect(safeSummary(row)).toContain('上游返回档位（最后候选）: Standard');
  delete row.evidence.upstream_service_tier;
  expect(safeSummary(row)).toContain('上游返回档位（最后候选）: 未知 / 未返回');
  row.evidence = {};
  expect(safeSummary(row)).toContain('请求档位: 未知 / 未返回');
});

it('reports Anchor policy independently from caller and upstream tiers', () => {
  const row = requestLog({ ...fixture.page.requests[0], evidence: {
    requested_service_tier: 'unspecified', mapping_service_tier: 'ultrafast', upstream_service_tier: 'default',
  } } as Request);
  expect(safeSummary(row)).toContain('请求档位: 未指定');
  expect(safeSummary(row)).toContain('Anchor 固定档位: Ultrafast');
  expect(safeSummary(row)).toContain('上游返回档位（最后候选）: Standard');
});
