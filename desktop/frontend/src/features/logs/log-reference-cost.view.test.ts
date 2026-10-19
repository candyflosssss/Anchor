import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import fixture from '../../../../contract/logs.json';
import type { CostAdjustment, ReferenceCost } from '../../desktop/contract.generated';
import { CostBreakdown, CostCell, CostSummary } from './log-reference-cost.view';

const adjustment = fixture.page.requests[1].reference_cost!.adjustment as CostAdjustment;
const render = (value: CostAdjustment) => renderToStaticMarkup(createElement(CostBreakdown, { adjustment: value }));
it('displays backend charging buckets, rates, multipliers and both amounts without deriving billing in the view', () => {
  const html = render(adjustment);
  for (const value of ['非缓存输入', '缓存读取', '缓存写入', '普通输出', '思考输出', '×0.5', '×0.8', '×0.1', 'USD / 百万 Token', '$0.00003', '$0.000012', '$0.000492']) expect(html).toContain(value);
});
it('discloses stale group observations, mixed retries and incomplete estimates rather than claiming one root multiplier', () => {
  expect(render({ ...adjustment, failure: 'network' })).toContain('最近刷新失败：network（保留上次观测）');
  const unknown = render({ ...adjustment, usd: null, reason: 'missing_multiplier', group_multiplier: null });
  expect(unknown).toContain('最终参考费用：未知');
  expect(unknown).toContain('分组 未知');
  const mixed = render({ ...adjustment, supplier_mixed: true, supplier_multiplier: null, group_mixed: true, group_multiplier: null });
  expect(mixed).toContain('供应商 逐次');
  expect(mixed).toContain('多次尝试分别计算后求和');
  expect(mixed).not.toContain('观测时间');
});
it('keeps unreported reasoning unknown and annotates the output bucket that includes it', () => {
  const parts = adjustment.parts.map(part => part.kind === 'output' ? { ...part, includes_reasoning: true } : part.kind === 'reasoning' ? { ...part, tokens: null } : part);
  expect(render({ ...adjustment, parts })).toContain('输出（含未拆分思考）');
  expect(render({ ...adjustment, parts })).toContain('未知 Token');
});

it('keeps four labelled values together in calculation order and preserves small amounts', () => {
  const html = renderToStaticMarkup(createElement(CostSummary, { usd: '0.000000000001', adjustment }));
  expect(html).toContain('$0.000000000001');
  const labels = ['原始费用', '供应商倍率', '分组倍率', '最终费用'].map(label => html.indexOf(label));
  expect(labels.every(index => index >= 0)).toBe(true);
  expect(labels).toEqual([...labels].sort((a, b) => a - b));
  expect(html).toContain('log-cost-final');
});
it('keeps absent prices, legacy snapshots, unknown factors, mixed retries and confirmed zero distinct', () => {
  const cell = (cost?: ReferenceCost | null) => renderToStaticMarkup(createElement(CostCell, { cost }));
  const cost = fixture.page.requests[1].reference_cost as ReferenceCost;
  expect(cell(null)).toBe('<span class="mono">—</span>');
  const legacy = cell({ ...cost, adjustment: null });
  expect(legacy).toContain('$0.00123');
  expect(legacy.match(/未记录/g)).toHaveLength(3);
  const unknown = cell({ ...cost, usd: null, adjustment: { ...adjustment, usd: null, group_multiplier: null } });
  expect(unknown.match(/未知/g)).toHaveLength(3);
  expect(unknown).not.toContain('$0');
  const mixed = cell({ ...cost, adjustment: { ...adjustment, supplier_mixed: true, group_mixed: true } });
  expect(mixed.match(/逐次/g)).toHaveLength(2);
  expect(mixed).toContain('$0.000492');
  const zero = cell({ ...cost, usd: '0', adjustment: { ...adjustment, usd: '0', supplier_multiplier: '0', group_multiplier: '0' } });
  expect(zero.match(/>\$0</g)).toHaveLength(2);
  expect(zero.match(/>×0</g)).toHaveLength(2);
});
