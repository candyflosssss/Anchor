import { expect, it } from 'vitest';
import fixture from '../../../../contract/logs.json';
import type { Request } from '../../desktop/contract.generated';
import { requestLog } from './log.service';
import { safeSummary, serviceTierLabel } from './log.model';
import { accelerationSummary, tierSnapshot } from './log-service-tier.model';

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

// Raw metadata and parsed annotations are deliberately separate.
it('retains unknown raw scalars without pretending they are known tiers', () => {
  for (const raw of ['"turbo-v2"', '"unknown"', '"unspecified"', '""', 'null', 'false', '42']) {
    const evidence = { upstream_service_tier: 'unknown', upstream_service_tier_raw: raw };
    expect(tierSnapshot(evidence, 'upstream').raw).toBe(raw);
    expect(accelerationSummary(evidence).state).toBe('unknown');
  }
  for (const raw of ['{"tier":"fast"}', 'broken', 'x'.repeat(257), 42, false]) {
    expect(accelerationSummary({ upstream_service_tier: 'priority', upstream_service_tier_raw: raw }).state).toBe('unknown');
  }
});
it('only confirms observed acceleration, detects lower/mismatched tiers and accepts aliases', () => {
  const cases = [
    ['priority', 'fast', 'confirmed', '已确认加速'],
    ['ultrafast', 'ultrafast', 'confirmed', '已确认加速'],
    ['ultrafast', 'priority', 'mismatch', '仅确认 Fast'],
    ['priority', 'default', 'mismatch', '未按请求加速'],
    ['priority', 'auto', 'unknown', '无法确认加速'],
    ['priority', 'scale', 'mismatch', '未按请求加速'],
    ['auto', 'ultrafast', 'confirmed', '上游报告加速'],
    ['default', 'default', 'none', '非 Fast 档位'],
  ];
  for (const [sent, returned, state, label] of cases) {
    expect(accelerationSummary({ sent_service_tier_raw: JSON.stringify(sent), upstream_service_tier_raw: JSON.stringify(returned) })).toMatchObject({ state, label });
  }
  expect(accelerationSummary({ sent_service_tier: 'priority', fast_triggered: 'true' }).state).toBe('unknown');
});
it('separates no parameter, not dispatched, missing response and old records', () => {
  expect(tierSnapshot({ sent_service_tier: 'unspecified' }, 'sent').raw).toBe('未指定');
  expect(tierSnapshot({ sent_service_tier: 'not_sent' }, 'sent').raw).toBe('未发送');
  expect(tierSnapshot({ sent_service_tier: 'priority' }, 'upstream').raw).toBe('未返回');
  expect(tierSnapshot({}, 'sent').raw).toBe('未知 / 未记录');
  expect(accelerationSummary({ upstream_service_tier: 'ultrafast' }).tier.kind).toBe('ultrafast');
});
it('preserves shared raw tier evidence in list mapping and clipboard summary', () => {
  const row = requestLog(fixture.page.requests[1] as Request);
  expect(tierSnapshot(row.evidence, 'sent').raw).toBe('"ultrafast"');
  expect(accelerationSummary(row.evidence)).toMatchObject({ state: 'confirmed', tier: { kind: 'ultrafast' } });
  expect(safeSummary(row)).toContain('发送档位（最后候选）: "ultrafast"');
  expect(safeSummary(row)).toContain('已确认加速 · Ultrafast');
});
