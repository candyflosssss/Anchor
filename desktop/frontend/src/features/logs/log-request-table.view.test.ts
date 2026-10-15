import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { logColumns, columnPresets } from './log-columns.model';
import { ColumnValue } from './log-request-table.view';
import { requestFixtures } from './log.mock';
import type { RequestLog } from './log.model';

const row: RequestLog = {
  ...requestFixtures[1], input: 1200, output: 300, elapsed: 2100, stream: true, attemptCount: 1,
  metrics: { outputGenerationMs: 1200, nonReasoningOutputTokens: 240, outputIntervalComplete: true, timeToFirstTextMs: 500, generationMs: 1200, outputTextTokens: 240,
    textIntervalComplete: true, cacheReadTokens: 400, reasoningTokens: 60 },
};
const render = (id: Parameters<typeof ColumnValue>[0]['id'], value = row) => renderToStaticMarkup(createElement(ColumnValue, { id, row: value }));
describe('grouped request log values', () => {
  it('shows total TPS from total output including reasoning over total request time', () => {
    const html = render('timing');
    for (const value of ['总耗时', '首字', '输出', '总TPS', '2,100 ms', '500 ms', '1,200 ms', '142.9 t/s']) expect(html).toContain(value);
    const failed = render('timing', { ...row, status: 'failed', metrics: { ...row.metrics!, outputIntervalComplete: false } });
    expect(failed).not.toContain('142.9 t/s');
    expect(failed.match(/未知/g)).toHaveLength(2);
  });
  it('shows TPS independently of stream, first output and interval metrics', () => {
    const html = render('timing', { ...row, stream: false, metrics: undefined });
    expect(html).toContain('142.9 t/s');
    expect(html).toContain('2,100 ms');
    expect(html.match(/未知/g)).toHaveLength(2);
    expect(render('timing', { ...row, metrics: { ...row.metrics!, timeToFirstOutputMs: 0 } })).toContain('>0 ms<');
    expect(render('timing', { ...row, output: 0 })).toContain('0.0 t/s');
  });
  it('keeps output duration visible even if the root total is unknown', () => {
    const html = render('timing', { ...row, output: null });
    expect(html).toContain('1,200 ms');
    expect(html).not.toContain('t/s');
    expect(html.match(/未知/g)).toHaveLength(1);
  });
  it('groups exact token counts without adding cache or reasoning to the total', () => {
    const html = render('tokens');
    for (const value of ['输入', '输出', '缓存命中', '思考', '合计', '1,200', '300', '400', '60', '1,500']) expect(html).toContain(value);
    expect(html).not.toContain('1,960');
    expect(render('tokens', { ...row, attemptCount: 2 })).toContain('已知小计');
  });
  it('distinguishes zero from unknown and does not infer totals with missing usage', () => {
    const html = render('tokens', { ...row, input: 0, output: null, metrics: undefined });
    expect(html).toContain('>0</dd>');
    expect(html.match(/未知/g)).toHaveLength(4);
    expect(render('tokens', { ...row, input: 0, output: 0 })).not.toContain('未知');
  });
  it('pairs category colors with labels and decorative icons, while muting only unknown values', () => {
    const html = render('tokens');
    for (const tone of ['input', 'output', 'cache', 'reasoning']) expect(html).toContain(`class="log-tone-${tone}"`);
    expect(html.match(/aria-hidden="true"/g)).toHaveLength(4);
    expect(html).not.toContain('is-unknown');
    const missing = render('tokens', { ...row, input: 0, output: null, metrics: undefined });
    expect(missing).toContain('class="log-tone-input"');
    for (const tone of ['output', 'cache', 'reasoning']) expect(missing).toContain(`class="log-tone-${tone} is-unknown"`);
    expect(render('timing').match(/aria-hidden="true"/g)).toHaveLength(4);
  });
  it('retains model identity, effort, key label, independent results and diagnostics', () => {
    const value = { ...row, upstreamModel: 'actual-model', reasoningEffort: null, key: 'safe-key-label',
      status: 'failed' as const, http: 200, code: 'invalid_response', stage: 'response_validate', delivery: '未输出' };
    expect(render('model', value)).toContain('actual-model');
    expect(render('model', value)).toContain('未指定');
    expect(render('model', { ...value, reasoningEffort: undefined })).toContain('未知');
    expect(render('provider', value)).toContain('safe-key-label');
    expect(render('status', value)).toContain('校验失败');
    expect(render('status', value)).toContain('HTTP 200');
    expect(render('status', value)).toContain('invalid_response');
    expect(render('status', value)).toContain('未输出');
    expect(render('diagnostics', value)).toContain('invalid_response');
    expect(render('diagnostics', value)).toContain('response_validate');
    expect(render('protocol', { ...value, stream: false })).toContain('非流式');
  });
  it('exposes compact block presets with no duplicate columns', () => {
    expect(logColumns).toHaveLength(12);
    expect(columnPresets.common.ids).toHaveLength(6);
    for (const preset of Object.values(columnPresets)) expect(new Set(preset.ids).size).toBe(preset.ids.length);
    for (const column of logColumns) expect(render(column.id)).not.toBe('');
  });
});
