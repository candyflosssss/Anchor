import { expect, test, type Page } from '@playwright/test';
import { resolve } from 'node:path';
async function ready(page: Page) {
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
}
async function edit(page: Page, id = '摸鱼AI') {
  await page.getByRole('button', { name: `编辑 ${id}`, exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
}
const scenario = async (page: Page, value: string) => {
  await page.getByLabel('原型场景').selectOption(value);
  await expect(page.getByText('正在读取供应商', { exact: true })).toBeHidden();
};
test('list, selection, search, filters and scope are truthful', async ({ page }) => {
  await ready(page);
  await expect(page.getByText('本机实例 · 模拟，未连接')).toBeVisible();
  await expect(page.getByText('摸鱼AI', { exact: true })).toHaveCount(1);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('revision 3');
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('上游连通性未测试');
  await page.getByLabel('搜索供应商名称或地址').fill('127.0.0.1');
  await expect(page.getByRole('button', { name: '本地调试', exact: true })).toBeVisible();
  await expect(page.getByLabel('摸鱼AI 的详情')).toHaveCount(0);
  await page.getByLabel('搜索供应商名称或地址').fill('no-such-value');
  await expect(page.getByRole('heading', { name: '没有匹配的供应商' })).toBeVisible();
  await page.getByRole('button', { name: '清除筛选' }).click();
  await page.getByRole('button', { name: '已停用 1', exact: true }).click();
  await expect(page.getByRole('button', { name: '本地调试', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toHaveCount(0);
});
test('new provider validates fields, handles Chinese ID and never persists secret', async ({ page }) => {
  const outside: string[] = [];
  page.on('request', request => { if (!request.url().startsWith('http://127.0.0.1:14201')) outside.push(request.url()); });
  await ready(page);
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(page.getByLabel('供应商名称 / ID')).toBeFocused();
  await page.getByLabel('供应商名称 / ID').fill('中文供应商');
  await page.getByLabel('API 根地址', { exact: true }).fill('https://new.example.com/v1');
  await page.getByLabel('API 密钥（可选）').fill('browser-test-only');
  await dialog.getByRole('button', { name: '添加供应商', exact: true }).dblclick();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('status')).toContainText('revision 1');
  await page.getByRole('button', { name: '中文供应商', exact: true }).click();
  await expect(page.getByLabel('中文供应商 的详情')).toContainText('已配置');
  const storage = await page.evaluate(() => ({ local: { ...localStorage }, session: { ...sessionStorage } }));
  expect(JSON.stringify(storage)).not.toContain('browser-test-only');
  expect(Object.keys(storage.local)).toEqual(['anchor.ui.appearance']);
  expect(storage.session).toEqual({}); expect(outside).toEqual([]);
  await page.reload();
  await expect(page.getByRole('button', { name: '中文供应商', exact: true })).toHaveCount(0);
});
test('complete edit preserves disabled status, limits and keep-secret semantics', async ({ page }) => {
  await ready(page); await edit(page, '本地调试');
  await expect(page.getByLabel('供应商名称 / ID')).toHaveAttribute('readonly', '');
  await expect(page.getByRole('switch', { name: '启用此供应商' })).not.toBeChecked();
  await expect(page.getByLabel('请求超时（秒）')).toHaveValue('30');
  await page.getByLabel('最大并发').fill('3');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await page.getByRole('button', { name: '本地调试', exact: true }).click();
  const detail = page.getByLabel('本地调试 的详情');
  await expect(detail).toContainText('已停用'); await expect(detail).toContainText('30 秒');
  await expect(detail).toContainText('3 个请求'); await expect(detail).toContainText('revision 3');
  await expect(detail).toContainText('未配置密钥');
});
test('discard prompt, escape, keyboard trap and focus restoration', async ({ page }) => {
  await ready(page); const trigger = page.getByRole('button', { name: '编辑 摸鱼AI', exact: true });
  await trigger.click(); await expect(page.getByLabel('供应商名称 / ID')).toBeFocused();
  await page.getByRole('button', { name: '替换密钥' }).click();
  await page.getByLabel('新的 API 密钥').fill('discard-test-only');
  await page.keyboard.press('Escape'); await expect(page.getByText('有未保存的修改', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '继续编辑' }).click();
  await expect(page.getByLabel('新的 API 密钥')).toHaveValue('discard-test-only');
  for (let i = 0; i < 18; i++) { await page.keyboard.press('Tab'); expect(await page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]'))).toBe(true); }
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await page.getByRole('button', { name: '丢弃并关闭' }).click();
  await expect(trigger).toBeFocused();
  await trigger.click(); await page.getByRole('button', { name: '替换密钥' }).click();
  await expect(page.getByLabel('新的 API 密钥')).toHaveValue('');
});
test('Chinese IME Enter does not submit the form', async ({ page }) => {
  await ready(page); await edit(page);
  const canceled = await page.getByLabel('API 根地址', { exact: true }).evaluate(element => !element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })));
  expect(canceled).toBe(true); await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('button', { name: '保存配置', exact: true })).toBeEnabled();
});
test('secret replacement is only-input and clearing requires explicit acknowledgement', async ({ page }) => {
  await ready(page); await edit(page);
  await page.getByRole('button', { name: '替换密钥' }).click();
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByText('替换密钥须非空', { exact: false })).toBeVisible();
  await page.getByLabel('新的 API 密钥').fill('replace-test-only');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await edit(page); await page.getByRole('button', { name: '清除密钥', exact: true }).click();
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByText('请确认清除密钥的影响。', { exact: true })).toBeVisible();
  await page.getByLabel('我确认清除 摸鱼AI 的密钥').check();
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('未配置密钥');
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('revision 5');
});
test('toggle confirms object and impact; cancel is the default focused action', async ({ page }) => {
  await ready(page); await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('button', { name: '停用供应商', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('停用 摸鱼AI？'); await expect(confirm).toContainText('配置和已存密钥会保留');
  await expect(confirm.getByRole('button', { name: '取消' })).toBeFocused();
  await confirm.getByRole('button', { name: '取消' }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('已启用');
  await page.getByRole('button', { name: '停用供应商', exact: true }).click();
  await confirm.getByRole('button', { name: '确认变更' }).dblclick();
  await expect(confirm).toBeHidden();
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('已停用');
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('revision 4');
});
test('conflict does not override latest values or silently retry', async ({ page }) => {
  await ready(page); await scenario(page, 'conflict'); await edit(page);
  await page.getByLabel('请求超时（秒）').fill('240');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('未覆盖新版本');
  await expect(page.getByRole('button', { name: '保存配置', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '丢弃草稿并重新读取' }).click();
  await expect(page.getByRole('dialog')).toBeHidden(); await edit(page);
  await expect(page.getByLabel('请求超时（秒）')).toHaveValue('180');
  await expect(page.getByRole('dialog')).toContainText('当前 revision 4');
});
test('unknown write locks mutations until explicit reread; no success toast', async ({ page }) => {
  await ready(page); await scenario(page, 'unknown'); await edit(page);
  await page.getByLabel('请求超时（秒）').fill('150');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByRole('alert')).toContainText('写入结果未知');
  await expect(page.getByRole('button', { name: '添加供应商', exact: true })).toBeDisabled();
  await expect(page.getByRole('status')).toHaveCount(0);
  await page.getByRole('button', { name: '重新读取配置', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('150 秒');
  await expect(page.getByLabel('摸鱼AI 的详情')).toContainText('revision 4');
});
test('empty, offline, unauthorized and read-error are distinct; stale loads discarded', async ({ page }) => {
  await ready(page); await scenario(page, 'empty');
  await expect(page.getByRole('heading', { name: '添加你的第一个供应商' })).toBeVisible();
  for (const value of ['offline', 'unauthorized', 'read_error']) {
    await scenario(page, value);
    await expect(page.getByRole('heading', { name: '暂时无法展示供应商' })).toBeVisible();
    await expect(page.getByRole('button', { name: '添加供应商', exact: true })).toBeDisabled();
    await expect(page.getByRole('alert')).toContainText(value === 'unauthorized' ? '管理授权不可用' : value === 'offline' ? '实例不可用' : '未能读取');
  }
  await page.getByLabel('原型场景').selectOption('offline');
  await page.getByLabel('原型场景').selectOption('normal');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});
test('appearance persists only a preference, follows system, works in narrow workspace', async ({ page }) => {
  await ready(page); await page.getByRole('button', { name: '深色', exact: true }).click();
  await expect(page.locator('html')).toHaveClass('dark'); await page.reload();
  await expect(page.locator('html')).toHaveClass('dark');
  await page.getByRole('button', { name: '跟随系统', exact: true }).click();
  await page.emulateMedia({ colorScheme: 'light' }); await expect(page.locator('html')).not.toHaveClass('dark');
  await page.emulateMedia({ colorScheme: 'dark' }); await expect(page.locator('html')).toHaveClass('dark');
  for (const width of [960, 720, 480]) {
    await page.setViewportSize({ width, height: 720 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.getByRole('button', { name: '添加供应商', exact: true }).click();
    await expect(page.getByRole('dialog').getByRole('button', { name: '添加供应商', exact: true })).toBeInViewport();
    await page.getByRole('button', { name: '取消', exact: true }).click();
  }
});
test('theme contrast and design captures', async ({ page }) => {
  await ready(page);
  for (const appearance of ['浅色', '深色']) {
    await page.getByRole('button', { name: appearance, exact: true }).click();
    const ratios = await page.evaluate(() => {
      const color = (token: string) => {
        const element = document.createElement('i'); element.style.color = `hsl(var(--${token}))`; document.body.append(element);
        const rgb = getComputedStyle(element).color.match(/[\d.]+/g)!.slice(0, 3).map(Number); element.remove();
        const linear = rgb.map(value => value / 255 <= .04045 ? value / 255 / 12.92 : ((value / 255 + .055) / 1.055) ** 2.4);
        return .2126 * linear[0] + .7152 * linear[1] + .0722 * linear[2];
      };
      const contrast = (a: string, b: string) => { const x = color(a), y = color(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05); };
      return { text: contrast('foreground', 'surface'), muted: contrast('muted-foreground', 'surface'), button: contrast('primary-foreground', 'primary'), input: contrast('input', 'surface'), focus: contrast('ring', 'surface'), success: contrast('success', 'surface') };
    });
    expect(ratios.text).toBeGreaterThanOrEqual(4.5); expect(ratios.muted).toBeGreaterThanOrEqual(4.5);
    expect(ratios.button).toBeGreaterThanOrEqual(4.5); expect(ratios.success).toBeGreaterThanOrEqual(4.5);
    expect(ratios.input).toBeGreaterThanOrEqual(3); expect(ratios.focus).toBeGreaterThanOrEqual(3);
    if (process.env.ANCHOR_CAPTURE === '1') await page.screenshot({ path: resolve('../../docs/design/previews', `providers-${appearance === '浅色' ? 'light' : 'dark'}.png`), fullPage: true });
  }
  if (process.env.ANCHOR_CAPTURE === '1') {
    await page.getByRole('button', { name: '浅色', exact: true }).click();
    await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
    await page.screenshot({ path: resolve('../../docs/design/previews/providers-detail.png'), fullPage: true });
    await edit(page);
    await page.screenshot({ path: resolve('../../docs/design/previews/providers-edit.png'), fullPage: false });
  }
});

test('official sidebar collapses with trigger and shortcut without persisting navigation', async ({ page }) => {
  await ready(page);
  const sidebar = page.locator('[data-variant="inset"][data-state]');
  const trigger = page.locator('[data-sidebar="trigger"]');
  await expect(sidebar).toHaveAttribute('data-state', 'expanded');
  await trigger.click(); await expect(sidebar).toHaveAttribute('data-state', 'collapsed');
  await page.keyboard.press('Control+b'); await expect(sidebar).toHaveAttribute('data-state', 'expanded');
  await edit(page);
  await page.keyboard.press('Control+b'); await expect(sidebar).toHaveAttribute('data-state', 'expanded');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toBeHidden();
  expect(await page.context().cookies()).toEqual([]);
  if (process.env.ANCHOR_CAPTURE === '1') {
    await trigger.click(); await expect(sidebar).toHaveAttribute('data-state', 'collapsed');
    await page.screenshot({ path: resolve('../../docs/design/previews/providers-collapsed.png') });
  }
});
test('mobile navigation is a closable sheet with keyboard focus restoration', async ({ page }) => {
  await page.setViewportSize({ width: 480, height: 800 });
  await ready(page);
  const trigger = page.locator('[data-sidebar="trigger"]');
  await trigger.click();
  const sheet = page.getByRole('dialog', { name: '工作台导航' });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('button', { name: '概览', exact: true })).toBeDisabled();
  await sheet.getByRole('button', { name: '深色', exact: true }).click();
  await expect(page.locator('html')).toHaveClass('dark');
  await page.keyboard.press('Escape');
  await expect(sheet).toBeHidden(); await expect(trigger).toBeFocused();
  await trigger.click(); await sheet.getByRole('button', { name: '关闭导航', exact: true }).click();
  await expect(sheet).toBeHidden(); await expect(trigger).toBeFocused();
  await trigger.click(); await sheet.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(sheet).toBeHidden(); await expect(trigger).toBeFocused();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (process.env.ANCHOR_CAPTURE === '1') await page.screenshot({ path: resolve('../../docs/design/previews/providers-mobile.png') });
});

test('long Chinese IDs and root URLs stay readable without widening the window', async ({ page }) => {
  await ready(page);
  const id = '供'.repeat(128);
  const url = `https://example.com/${'gateway/'.repeat(20)}v1`;
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  await page.getByLabel('供应商名称 / ID').fill(id);
  await page.getByLabel('API 根地址', { exact: true }).fill(url);
  await page.getByRole('dialog').getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  await page.getByRole('button', { name: id, exact: true }).click();
  const detail = page.getByLabel(`${id} 的详情`);
  await expect(detail).toContainText(url);
  for (const width of [960, 720, 480]) {
    await page.setViewportSize({ width, height: 800 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expect(detail.getByRole('button', { name: '编辑配置', exact: true })).toBeVisible();
  }
});
