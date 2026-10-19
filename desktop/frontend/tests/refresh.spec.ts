import { expect, test, type Page } from '@playwright/test';

declare global { interface Window { refreshTest: { lists: number; workspaces: string[]; balances: string[]; delay: number; fail: boolean } } }
async function instrument(page: Page) {
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
  await expect(page.getByText('读取中', { exact: true })).toHaveCount(0);
  await expect(page.getByText('查询中…', { exact: true })).toHaveCount(0);
  await page.evaluate(async () => {
    const modulePath = '/src/features/providers/provider.mock.ts';
    const { MockProviderTransport } = await import(modulePath) as typeof import('../src/features/providers/provider.mock');
    const state = window.refreshTest = { lists: 0, workspaces: [] as string[], balances: [] as string[], delay: 0, fail: false };
    const { list, workspace, allowance, discover } = MockProviderTransport.prototype;
    MockProviderTransport.prototype.list = async function (...args) {
      state.lists++; await new Promise(resolve => setTimeout(resolve, state.delay));
      if (state.fail) throw new Error('isolated list failure');
      return list.apply(this, args);
    };
    MockProviderTransport.prototype.discover = async function (...args) {
      await new Promise(resolve => setTimeout(resolve, state.delay));
      if (state.fail) throw new Error('isolated catalog failure');
      return discover.apply(this, args);
    };
    MockProviderTransport.prototype.workspace = async function (...args) { state.workspaces.push(args[0]); return workspace.apply(this, args); };
    MockProviderTransport.prototype.allowance = async function (...args) { state.balances.push(args[0].provider_id); return allowance.apply(this, args); };
  });
}

test('provider toggle refreshes only its own dependencies and keeps rows mounted', async ({ page }) => {
  await instrument(page);
  const other = page.getByRole('row').filter({ has: page.getByRole('button', { name: '备用供应商', exact: true }) });
  const row = await other.elementHandle();
  const before = await other.innerText();
  const position = await other.boundingBox();
  const toggle = page.getByRole('switch', { name: '供应商 摸鱼AI 启用状态', exact: true });
  await toggle.click(); await expect(toggle).not.toBeChecked(); await expect(toggle).toBeEnabled();
  await expect.poll(() => page.evaluate(() => window.refreshTest.workspaces)).toEqual(['摸鱼AI']);
  await expect.poll(() => page.evaluate(() => window.refreshTest.balances)).toEqual(['摸鱼AI']);
  expect(await page.evaluate(() => window.refreshTest.lists)).toBe(0);
  expect(await row!.evaluate(el => el.isConnected)).toBe(true);
  expect(await other.innerText()).toBe(before);
  expect((await other.boundingBox())?.y).toBe(position?.y);
});

test('manual refresh and refresh failure retain the table and allow explicit recovery', async ({ page }) => {
  await instrument(page);
  const row = await page.getByRole('row').filter({ has: page.getByRole('button', { name: '摸鱼AI', exact: true }) }).elementHandle();
  await page.evaluate(() => { window.refreshTest.delay = 800; window.refreshTest.fail = true; });
  const refresh = page.getByRole('button', { name: '刷新供应商列表' });
  await refresh.click(); await expect(refresh).toBeDisabled();
  expect(await row!.evaluate(el => el.isConnected)).toBe(true);
  await expect(page.getByText('正在读取供应商', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('alert')).toContainText('读取供应商失败');
  expect(await row!.evaluate(el => el.isConnected)).toBe(true);
  await expect(page.getByRole('switch', { name: '供应商 摸鱼AI 启用状态', exact: true })).toBeDisabled();
  await page.evaluate(() => { window.refreshTest.fail = false; });
  await page.getByRole('button', { name: '重新读取配置', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByRole('switch', { name: '供应商 摸鱼AI 启用状态', exact: true })).toBeEnabled();
  expect(await row!.evaluate(el => el.isConnected)).toBe(true);
});

test('Key toggle reconciles in place without collapsing the detail workspace', async ({ page }) => {
  await instrument(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^Key/ }).click();
  const toggle = page.getByRole('switch', { name: 'Key default 启用状态', exact: true });
  await expect(toggle).toBeEnabled();
  const element = await toggle.elementHandle();
  const position = await toggle.boundingBox();
  await page.evaluate(() => { window.refreshTest.delay = 800; });
  await toggle.click(); await expect(toggle).not.toBeChecked();
  await expect.poll(() => page.evaluate(() => window.refreshTest.lists)).toBe(1);
  expect(await element!.evaluate(el => el.isConnected)).toBe(true);
  await expect(toggle).toBeEnabled();
  expect(await element!.evaluate(el => el.isConnected)).toBe(true);
  await expect(page.getByText('正在读取供应商', { exact: true })).toHaveCount(0);
  expect((await toggle.boundingBox())?.y).toBe(position?.y);
});

test('revision-only account updates preserve form nodes, drafts and expanded sections', async ({ page }) => {
  await instrument(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额', exact: true }).click();
  const platform = page.getByLabel('平台类型', { exact: true });
  await expect(platform).toBeEnabled();
  const input = await platform.elementHandle();
  await page.getByText('账户接口地址（可选）', { exact: true }).click();
  await page.getByLabel('站点根地址', { exact: true }).fill('https://draft.example.test');
  await page.evaluate(() => { window.refreshTest.delay = 800; });
  await page.getByRole('switch', { name: '供应商 摸鱼AI 启用状态', exact: true }).click();
  expect(await input!.evaluate(el => el.isConnected)).toBe(true);
  await expect(platform).toBeEnabled();
  expect(await input!.evaluate(el => el.isConnected)).toBe(true);
  await expect(page.getByLabel('站点根地址', { exact: true })).toBeVisible();
  await expect(page.getByLabel('站点根地址', { exact: true })).toHaveValue('https://draft.example.test');
});

test('catalog refresh preserves results, disclosure and filters on success and failure', async ({ page }) => {
  await instrument(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  const catalog = page.getByRole('region', { name: '上游模型目录', exact: true });
  await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await expect(catalog).toContainText('已获取 2 个模型');
  await catalog.getByText('Key 查询详情', { exact: false }).click();
  await catalog.getByLabel('搜索模型目录').fill('demo');
  const details = await catalog.locator('details').elementHandle();
  const models = await catalog.getByLabel('上游模型 ID').elementHandle();
  for (const fail of [false, true]) {
    await page.evaluate(fail => { window.refreshTest.delay = 700; window.refreshTest.fail = fail; }, fail);
    await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
    await expect(catalog).toHaveAttribute('aria-busy', 'true');
    expect(await details!.evaluate(el => el.isConnected)).toBe(true);
    expect(await models!.evaluate(el => el.isConnected)).toBe(true);
    await expect(catalog).toContainText('已获取 2 个模型');
    await expect(catalog).toHaveAttribute('aria-busy', 'false');
    expect(await details!.evaluate(el => el.isConnected)).toBe(true);
    await expect(catalog.getByLabel('Key 查询结果')).toBeVisible();
    await expect(catalog.getByLabel('搜索模型目录')).toHaveValue('demo');
    if (fail) await expect(catalog.getByRole('alert')).toBeVisible();
  }
});
