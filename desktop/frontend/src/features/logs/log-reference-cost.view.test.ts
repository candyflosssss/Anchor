import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import fixture from '../../../../contract/logs.json';
import type { CostAdjustment } from '../../desktop/contract.generated';
import { CostBreakdown } from './log-reference-cost.view';

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
