import { expect, test, type Page } from '@playwright/test';
import { resolve } from 'node:path';
import fixture from '../../contract/logs.json' with { type: 'json' };
import type { DesktopBridge, Query } from '../src/desktop/contract.generated';
const capture = process.env.ANCHOR_LOG_SCREENSHOTS;
declare global { interface Window { logTest: { queries: Query[]; exports: number; settings: typeof fixture.settings; unavailable: boolean; paged: boolean } } }
async function openLogs(page: Page) {
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
  }, fixture);
  await page.goto('/');
  await page.getByRole('button', { name: '日志', exact: true }).click();
  await expect(page.getByRole('button', { name: '查看请求 fixture_failed' })).toBeVisible();
}
test('real-shaped filters, HTTP 200 validation failure, attempts and keyboard focus', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
  await openLogs(page);
  const list = page.getByRole('region', { name: '请求日志列表' });
  await expect(list.getByRole('row')).toHaveCount(3);
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
  await dialog.getByRole('checkbox', { name: /^供应商/ }).uncheck();
  await dialog.getByRole('checkbox', { name: /^思考深度/ }).check();
  await dialog.getByRole('button', { name: '左移思考深度', exact: true }).click();
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  await expect(opener).toBeFocused();
  await expect(list.getByRole('columnheader', { name: '供应商', exact: true })).toHaveCount(0);
  const headers = await list.getByRole('columnheader').allTextContents();
  expect(headers.indexOf('思考深度')).toBeLessThan(headers.indexOf('错误码'));
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) })).toContainText('high');
  expect(await page.evaluate(() => window.logTest.queries.every(q => !('columns' in q)))).toBe(true);
  await page.reload(); await page.getByRole('button', { name: '日志', exact: true }).click();
  await expect(list.getByRole('columnheader')).toHaveText(headers);
  await opener.click(); await dialog.getByRole('button', { name: '性能', exact: true }).click();
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-columns-panel.png') });
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  const success = page.getByRole('row').filter({ has: page.getByRole('button', { name: '查看请求 fixture_success' }) });
  await expect(success).toContainText('584 ms'); await expect(success).toContainText('66.7 t/s');
  await opener.click(); await dialog.getByRole('button', { name: '恢复默认', exact: true }).click();
  while (await dialog.getByRole('checkbox', { checked: true }).count() > 1) await dialog.getByRole('checkbox', { checked: true }).first().uncheck();
  await expect(dialog.getByRole('checkbox', { checked: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '恢复默认', exact: true }).click(); await page.keyboard.press('Escape');
  await expect(success).toContainText('provider-a');
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await opener.click(); expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  if (capture) await page.screenshot({ path: resolve(capture, 'logs-columns-mobile.png') });
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
