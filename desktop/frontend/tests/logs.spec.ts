import { expect, test, type Page } from '@playwright/test';
import { resolve } from 'node:path';
import fixture from '../../contract/logs.json' with { type: 'json' };
import type { DesktopBridge, Query } from '../src/desktop/contract.generated';
const capture = process.env.ANCHOR_LOG_SCREENSHOTS;
declare global { interface Window { logTest: { queries: Query[]; exports: number; settings: typeof fixture.settings; unavailable: boolean; paged: boolean } } }
async function openLogs(page: Page, data = fixture) {
  await page.addInitScript(f => {
    window.logTest = { queries: [], exports: 0, settings: f.settings, unavailable: false, paged: false };
    const result = <T,>(data: T) => ({ contract: f.contract, data, problem: null });
    const session = { instance_id: 'logs-isolated', phase: 'ready', source: 'started', can_stop: true };
    const rows = f.page.requests.map(r => ({ ...r, started_at_ms: Date.now() }));
    const api = {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      Logs: async (q: Query) => {
        window.logTest.queries.push(q);
        if (window.logTest.unavailable) return { contract: f.contract, data: null, problem: { code: 'offline' } };
        if (q.kind === 'system') return result(f.system);
        const selected = rows.filter(r => (!q.status || r.status === q.status) && (!q.source || r.source === q.source) && (!q.provider || r.provider === q.provider) && (!q.search || r.id.startsWith(q.search)));
        return result({ ...f.page, requests: selected.map(r => ({ ...r, attempts: [], evidence: {} })), latest: q.cursor ? 3 : 2, has_more: window.logTest.paged && !q.cursor, next_cursor: 1 });
      },
      LogDetail: async (id: string) => result(rows.find(r => r.id === id)),
      LogSettings: async () => result(window.logTest.settings),
      ConfigureLogs: async (s: typeof f.settings) => { window.logTest.settings = s; return result(s); },
      ExportLogs: async () => { window.logTest.exports++; return result({ accepted: true }); },
    } as unknown as DesktopBridge;
    window.go = { desktop: { Bridge: api } };
  }, data);
  await page.goto('/');
  await page.getByRole('button', { name: '日志', exact: true }).click();
  await expect(page.getByRole('button', { name: '查看请求 fixture_failed' })).toBeVisible();
}
test('real-shaped filters, HTTP 200 validation failure, attempts and keyboard focus', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
  await openLogs(page);
  const list = page.getByRole('region', { name: '请求日志列表' });
  await expect(list.getByRole('row')).toHaveCount(3);
  await expect(list.getByRole('columnheader')).toHaveText(['时间', '结果与交付', '模型', '供应商 / Key', '耗时与速度', 'Token 用量', '详情']);
  const success = list.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) });
  const timing = success.getByRole('cell').nth(4), tokens = success.getByRole('cell').nth(5);
  for (const label of ['总耗时', '首字', '输出', '总TPS']) await expect(timing.getByText(label, { exact: true })).toBeVisible();
  for (const label of ['输入', '输出', '缓存命中', '思考', '合计']) await expect(tokens.getByText(label, { exact: true })).toBeVisible();
  await expect(success.getByRole('cell').nth(1)).toContainText('HTTP 200');
  await expect(list.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_failed' }) }).getByRole('cell').nth(1)).toContainText('invalid_response');
  await expect(success.getByRole('cell').nth(2)).toContainText('high');
  const bounds = await success.evaluate(row => {
    const time = row.querySelector('.log-time')!.getBoundingClientRect();
    const cell = row.querySelector('td')!.getBoundingClientRect();
    return { rowHeight: row.getBoundingClientRect().height, timeRight: time.right, cellRight: cell.right,
      timeWidth: row.querySelector('.log-time')!.scrollWidth, availableWidth: row.querySelector('.log-time')!.clientWidth };
  });
  expect(bounds.timeWidth).toBeLessThanOrEqual(bounds.availableWidth);
  expect(bounds.timeRight).toBeLessThanOrEqual(bounds.cellRight);
  expect(bounds.rowHeight).toBeLessThan(140);
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-light.png'), fullPage: true });
  await page.getByLabel('请求结果', { exact: true }).selectOption('failed');
  await expect(list.getByRole('row')).toHaveCount(2);
  const opener = page.getByRole('button', { name: '查看请求 fixture_failed' });
  await opener.focus(); await page.keyboard.press('Enter');
  const detail = page.getByRole('dialog');
  await expect(detail).toContainText('invalid_response');
  await expect(detail).toContainText('response_validate');
  await expect(detail).toContainText('200');
  await expect(detail).toContainText('json_valid');
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-failure-detail.png') });
  await page.keyboard.press('Escape'); await expect(detail).toBeHidden(); await expect(opener).toBeFocused();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await page.getByLabel('供应商筛选').fill('provider-a');
  await expect(page.getByRole('button', { name: '查看请求 fixture_success' })).toBeVisible();
  await page.getByLabel('搜索日志').fill('no-such-request'); await expect(page.getByText('暂无日志', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await page.getByRole('button', { name: '深色', exact: true }).click();
  await expect(page.locator('html')).toHaveClass(/dark/);
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-dark.png'), fullPage: true });
  expect(errors).toEqual([]);
});
test('column preferences preserve collected values and survive reload', async ({ page }) => {
  await openLogs(page);
  const list = page.getByRole('region', { name: '请求日志列表' });
  const opener = page.getByRole('button', { name: /显示内容/ });
  await opener.click(); const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox', { name: /^供应商 \/ Key/ }).uncheck();
  await dialog.getByRole('checkbox', { name: /^请求方式/ }).check();
  await dialog.getByRole('button', { name: '左移请求方式', exact: true }).click();
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  await expect(opener).toBeFocused();
  await expect(list.getByRole('columnheader', { name: '供应商 / Key', exact: true })).toHaveCount(0);
  const headers = await list.getByRole('columnheader').allTextContents();
  expect(headers.indexOf('请求方式')).toBeLessThan(headers.indexOf('Token 用量'));
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) })).toContainText('high');
  expect(await page.evaluate(() => window.logTest.queries.every(q => !('columns' in q)))).toBe(true);
  await page.reload(); await page.getByRole('button', { name: '日志', exact: true }).click();
  await expect(list.getByRole('columnheader')).toHaveText(headers);
  await opener.click(); await dialog.getByRole('button', { name: '性能', exact: true }).click();
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-columns-panel.png') });
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  const success = page.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) });
  await expect(success).toContainText('584 ms'); await expect(success).toContainText('57.1 t/s');
  await opener.click(); await dialog.getByRole('button', { name: '恢复默认', exact: true }).click();
  while (await dialog.getByRole('checkbox', { checked: true }).count() > 1) await dialog.getByRole('checkbox', { checked: true }).first().uncheck();
  await expect(dialog.getByRole('checkbox', { checked: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '恢复默认', exact: true }).click(); await page.keyboard.press('Escape');
  await expect(success).toContainText('provider-a');
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.getByRole('button', { name: '查看请求 fixture_success' }).click();
  await expect(dialog.getByLabel('Token 用量')).toBeVisible();
  expect(await dialog.getByLabel('Token 用量').evaluate(el => el.clientWidth)).toBeGreaterThan(200);
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-detail-mobile.png') });
  await page.keyboard.press('Escape');
  await opener.click(); expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-columns-mobile.png') });
});
test('legacy individual column preferences migrate into ordered blocks', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('anchor.ui.logs.columns.v1', JSON.stringify([
    'time', 'ttft', 'elapsed', 'generationMs', 'tps', 'input', 'output', 'cacheReadTokens', 'reasoningTokens', 'model', 'reasoningEffort', 'provider', 'key', 'http', 'delivery', 'errorCode', 'stage',
  ])));
  await openLogs(page);
  const list = page.getByRole('region', { name: '请求日志列表' });
  await expect(list.getByRole('columnheader')).toHaveText(['时间', '耗时与速度', 'Token 用量', '模型', '供应商 / Key', '结果与交付', '错误与阶段', '详情']);
  const failed = list.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_failed' }) });
  await expect(failed).toContainText('invalid_response');
  await expect(failed).toContainText('response_validate');
  await expect(failed).toContainText('HTTP 200');
});
test('retention, export, stable cursor snapshot and system correlation', async ({ page }) => {
  await openLogs(page);
  await page.evaluate(() => { window.logTest.paged = true; }); await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByRole('button', { name: '下一页日志' }).click(); await expect(page.getByText('第 2 页', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.logTest.queries.at(-1)?.until)).toBe(2);
  await page.getByRole('button', { name: '上一页日志' }).click();
  await page.getByRole('button', { name: '日志保留设置' }).click();
  const dialog = page.getByRole('dialog'); await dialog.getByLabel('请求保留天数').fill('14'); await dialog.getByRole('button', { name: '保存', exact: true }).click();
  await expect(dialog).toBeHidden(); expect(await page.evaluate(() => window.logTest.settings.days)).toBe(14);
  await page.getByRole('button', { name: '导出', exact: true }).click(); await expect(page.getByText('已导出', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.logTest.exports)).toBe(1);
  await page.getByRole('button', { name: '系统日志', exact: true }).click();
  await page.getByRole('button', { name: 'fixture_failed', exact: true }).click(); await expect(dialog).toContainText('invalid_response'); await page.keyboard.press('Escape');
  await page.evaluate(() => { window.logTest.unavailable = true; }); await page.getByRole('button', { name: '刷新', exact: true }).click(); await expect(page.getByText('日志不可用', { exact: true })).toBeVisible();
});

test('colored metric tiles stay readable with realistic counts in light and dark themes', async ({ page }) => {
  const data = structuredClone(fixture);
  const request = data.page.requests.find(row => row.id === 'fixture_success')!;
  Object.assign(request, { input: 104823, output: 2847, elapsed_ms: 10883, upstream_model: 'claude-sonnet-4-5-20250929' });
  Object.assign(request.metrics, { cache_read_tokens: 98304, reasoning_tokens: 1200, time_to_first_output_ms: 1823,
    output_generation_ms: 9060, non_reasoning_output_tokens: 1647 });
  await openLogs(page, data);
  const success = page.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) });
  await expect(success).toContainText('104,823');
  await expect(success).toContainText('10,883 ms');
  for (const theme of ['light', 'dark']) {
    await page.evaluate(dark => document.documentElement.classList.toggle('dark', dark), theme === 'dark');
    const styles = await success.locator('.log-token-block .log-metric-grid > div').evaluateAll(tiles => tiles.map(tile => ({
      color: getComputedStyle(tile).color, background: getComputedStyle(tile).backgroundColor,
      label: tile.querySelector('dt')!.textContent,
      fits: [...tile.querySelectorAll('dt, dd')].every(el => el.scrollWidth <= el.clientWidth && el.getBoundingClientRect().height < 22),
    })));
    expect(new Set(styles.map(style => style.color)).size).toBe(4);
    expect(styles.every(style => style.background !== 'rgba(0, 0, 0, 0)' && style.fits)).toBe(true);
    expect(styles.map(style => style.label)).toEqual(['输入', '输出', '缓存命中', '思考']);
    expect(await success.evaluate(el => el.getBoundingClientRect().height)).toBeLessThan(140);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    if (capture) await page.screenshot({ path: resolve(capture, `logs-metrics-${theme}.png`), fullPage: true });
  }
});

test('display-content action keeps its outline separate from the tab underline', async ({ page }) => {
  await openLogs(page);
  const opener = page.getByRole('button', { name: /显示内容/ });
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const theme of ['light', 'dark']) {
      await page.evaluate(dark => document.documentElement.classList.toggle('dark', dark), theme === 'dark');
      await opener.focus();
      await expect(opener).toBeFocused();
      const style = await opener.evaluate(button => {
        const css = getComputedStyle(button);
        const bounds = button.getBoundingClientRect();
        const row = button.closest('.log-tabs-row')!.getBoundingClientRect();
        return { bottomColor: css.borderBottomColor, topColor: css.borderTopColor,
          bottomWidth: css.borderBottomWidth, topWidth: css.borderTopWidth,
          marginBottom: css.marginBottom, bottomGap: row.bottom - bounds.bottom };
      });
      expect(style.bottomColor).toBe(style.topColor);
      expect(style.bottomWidth).toBe('1px');
      expect(style.bottomWidth).toBe(style.topWidth);
      expect(style.marginBottom).toBe('0px');
      expect(style.bottomGap).toBeGreaterThanOrEqual(4);
      if (capture) await page.locator('.log-tabs-row').screenshot({ path: resolve(capture, `logs-display-button-${width}-${theme}.png`) });
      await page.keyboard.press('Enter');
      await expect(page.getByRole('dialog', { name: '显示内容' })).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(opener).toBeFocused();
    }
  }
});
