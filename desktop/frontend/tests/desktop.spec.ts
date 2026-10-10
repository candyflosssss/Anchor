import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import { CONTRACT, type DesktopBridge, type Provider } from '../src/desktop/contract.generated';

declare global { interface Window { desktopTest: { starts: number; saves: number; stops: number; status: boolean; writeError: 'conflict' | 'unknown' | null } } }
async function host(page: Page) {
  // A fake Wails boundary exercises the real frontend mode, not a native window.
  await page.addInitScript(({ contract, rows }) => {
    window.desktopTest = { starts: 0, saves: 0, stops: 0, status: true, writeError: null };
    const counters = window.desktopTest;
    let providers: Provider[] = rows;
    const data = { instance_id: 'test-local-instance', phase: 'ready', source: 'started', can_stop: true };
    const api: DesktopBridge = {
      Start: async () => { counters.starts++; return { contract, data, problem: null }; },
      Status: async () => ({ contract, data: counters.status ? data : null, problem: counters.status ? null : { code: 'offline' } }),
      Providers: async () => ({ contract, data: providers, problem: null }),
      SaveProvider: async input => {
        counters.saves++;
        if (counters.writeError) return { contract, data: null, problem: { code: counters.writeError } };
        const saved = { ...input.provider, revision: input.expected_revision + 1, has_secret: input.secret === null ? input.provider.has_secret : !!input.secret };
        providers = [...providers.filter(p => p.id !== saved.id), saved];
        return { contract, data: saved, problem: null };
      },
      StopCore: async input => {
        if (!input.approved || input.instance_id !== data.instance_id) return { contract, data: null, problem: { code: 'invalid' } };
        counters.stops++; return { contract, data: { accepted: true }, problem: null };
      },
    };
    window.go = { desktop: { Bridge: api } };
  }, { contract: CONTRACT, rows: fixture.providers });
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
}
test('desktop browser without Wails bindings fails closed instead of showing mock data', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible();
  await expect(page.getByText('桌面接口不可用或版本不兼容。', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toHaveCount(0);
  await expect(page.getByLabel('原型场景')).toHaveCount(0);
});
test('desktop starts once, loads real-mode data and submits without persisting secrets', async ({ page }) => {
  await host(page);
  expect(await page.evaluate(() => window.desktopTest.starts)).toBe(1);
  await expect(page.getByText('原型场景', { exact: true })).toHaveCount(0);
  await expect(page.getByText('本机实例 · 模拟，未连接', { exact: true })).toHaveCount(0);
  await expect(page.getByText('本次启动 · 关闭窗口不会停止核心 · 不自动设置开机自启动')).toBeVisible();
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(page.getByRole('button', { name: '停止核心', exact: true, includeHidden: true })).toBeDisabled();
  await page.getByLabel('供应商名称 / ID').fill('桌面供应商');
  await page.getByLabel('API 根地址', { exact: true }).fill('https://desktop.example.com/v1');
  await page.getByLabel('API 密钥（可选）').fill('desktop-synthetic-secret');
  await page.getByRole('dialog').getByRole('button', { name: '添加供应商', exact: true }).dblclick();
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByRole('button', { name: '桌面供应商', exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('desktop-synthetic-secret');
  await expect(page.getByText('演示配置已保存', { exact: false })).toHaveCount(0);
});
test('stop is separately confirmed, targeted and only reports acceptance', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '停止核心', exact: true }).click();
  const confirmation = page.getByRole('alertdialog');
  await expect(confirmation).toContainText('test-local-instance');
  await expect(confirmation).toContainText('本次由桌面宿主启动');
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.desktopTest.stops)).toBe(0);
  await page.getByRole('button', { name: '停止核心', exact: true }).click();
  await confirmation.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByRole('heading', { name: '停服请求已接受' })).toBeVisible();
  await expect(page.getByText('核心正在处理退出与配置恢复；请求接受不代表恢复已完成。')).toBeVisible();
  expect(await page.evaluate(() => ({ stops: window.desktopTest.stops, starts: window.desktopTest.starts }))).toEqual({ stops: 1, starts: 1 });
});
test('disconnect clears open secret drafts and never silently restarts the core', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '编辑 摸鱼AI', exact: true }).click();
  await page.getByRole('button', { name: '替换密钥', exact: true }).click();
  await page.getByLabel('新的 API 密钥').fill('disconnect-synthetic-secret');
  await page.evaluate(() => { window.desktopTest.status = false; });
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible({ timeout: 8000 });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(await page.evaluate(() => ({ starts: window.desktopTest.starts, saves: window.desktopTest.saves }))).toEqual({ starts: 1, saves: 0 });
  expect(await page.content()).not.toContain('disconnect-synthetic-secret');
});
test('a structured write conflict does not replay or claim success', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.desktopTest.writeError = 'conflict'; });
  await page.getByRole('button', { name: '编辑 摸鱼AI', exact: true }).click();
  await page.getByLabel('最大并发').fill('6');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog').getByText('配置已被其他操作修改。未覆盖新版本；请重新读取后再编辑。')).toBeVisible();
  await expect(page.getByRole('button', { name: '保存配置', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
});

test('polling refreshes the authenticated source and stop capability without restarting', async ({ page }) => {
  await host(page);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!;
    const previous = api.Status;
    api.Status = async () => {
      const result = await previous();
      return { ...result, data: result.data ? { ...result.data, source: 'reused', can_stop: false } : null };
    };
  });
  await expect(page.getByText('复用已有核心 · 关闭窗口不会停止核心 · 不自动设置开机自启动')).toBeVisible({ timeout: 8000 });
  await expect(page.getByRole('button', { name: '停止核心', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.desktopTest.starts)).toBe(1);
});
