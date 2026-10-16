import { expect, test, type Page } from '@playwright/test';

const trigger = (page: Page) => page.locator('[data-sidebar=trigger]');
async function ready(page: Page) {
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
  await expect(page.locator('.health-pill').filter({ hasText: '读取中' })).toHaveCount(0);
}
async function fits(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}

test('sidebar and settings share a persistent, system-aware appearance preference', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await ready(page);
  await trigger(page).click();
  await expect(trigger(page)).toHaveAttribute('aria-expanded', 'false');
  for (const button of await page.locator('.appearance-control button').all()) {
    const box = await button.boundingBox();
    expect(box!.width).toBeGreaterThanOrEqual(32);
    expect(box!.height).toBeGreaterThanOrEqual(32);
  }
  await page.getByRole('button', { name: '深色', exact: true }).click();
  await expect(page.locator('html')).toHaveClass('dark');
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await expect(page.getByRole('heading', { level: 1, name: '设置', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { level: 2, name: '在线更新', exact: true })).toBeVisible();
  const options = page.locator('.appearance-options');
  await expect(options.getByRole('button', { name: '深色', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await options.getByRole('button', { name: '跟随系统', exact: true }).click();
  await expect(page.locator('html')).not.toHaveClass('dark');
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveClass('dark');
  await options.getByRole('button', { name: '浅色', exact: true }).focus();
  await page.keyboard.press('Space');
  await expect(page.locator('html')).not.toHaveClass('dark');
  await expect(page.locator('.appearance-control').getByRole('button', { name: '浅色', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.reload();
  await expect(page.locator('html')).not.toHaveClass('dark');
  expect(await page.evaluate(() => localStorage.getItem('anchor.ui.appearance'))).toBe('light');
});

test('toolbars fit desktop breakpoints and tables scroll locally', async ({ page }) => {
  await ready(page);
  for (const width of [1280, 1024, 900, 800, 768, 720, 480]) {
    await page.setViewportSize({ width, height: 800 });
    await fits(page);
    const search = page.getByRole('textbox', { name: '搜索供应商名称或地址' });
    const box = await search.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    await expect(search).toBeVisible();
    await expect(page.getByRole('button', { name: '添加供应商', exact: true })).toBeVisible();
  }
  expect(await page.locator('.table-scroll > div').evaluate(e => e.scrollWidth > e.clientWidth)).toBe(true);
  await page.setViewportSize({ width: 800, height: 420 });
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
  const header = page.locator('.app-titlebar');
  expect((await header.boundingBox())!.y).toBeGreaterThanOrEqual(0);
  expect((await header.boundingBox())!.height).toBe(40);
  await expect(trigger(page)).toBeInViewport();
});

test('mobile navigation restores focus and appearance choices remain reachable', async ({ page }) => {
  await page.setViewportSize({ width: 480, height: 640 });
  await ready(page);
  await trigger(page).click();
  const drawer = page.getByRole('dialog', { name: '工作台导航', exact: true });
  await expect(drawer).toBeVisible();
  await drawer.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await expect(drawer).toBeHidden();
  await expect(trigger(page)).toBeFocused();
  await fits(page);
  for (const name of ['浅色', '深色', '跟随系统']) await expect(page.locator('.appearance-options').getByRole('button', { name, exact: true })).toBeVisible();
  await trigger(page).click();
  await page.keyboard.press('Escape');
  await expect(drawer).toBeHidden();
  await expect(trigger(page)).toBeFocused();
});

test('primary actions are unique and failed overview reads are not measured zeros', async ({ page }) => {
  await ready(page);
  const primaries = page.locator('main button.bg-primary:visible');
  await expect(primaries).toHaveCount(1);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toBeVisible();
  await expect(primaries).toHaveCount(1);
  await page.getByRole('button', { name: '概览', exact: true }).click();
  await expect(primaries).toHaveCount(1);
  await page.getByLabel('原型场景').selectOption('read_error');
  await expect(page.getByRole('alert')).toBeVisible();
  const metrics = page.getByRole('region', { name: '关键指标' });
  await expect(metrics.locator('.metric-value')).toHaveText(['—', '—', '—', '—']);
  await expect(metrics).not.toContainText('无失败记录');
  await page.getByLabel('原型场景').selectOption('empty');
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(page.getByRole('heading', { name: '添加你的第一个供应商', exact: true })).toBeVisible();
  await expect(primaries).toHaveCount(1);
});

test('sidebar shortcut ignores in-progress IME composition', async ({ page }) => {
  await ready(page);
  await page.locator('body').dispatchEvent('keydown', { key: 'b', ctrlKey: true, isComposing: true });
  await expect(trigger(page)).toHaveAttribute('aria-expanded', 'true');
  await page.keyboard.press('Control+b');
  await expect(trigger(page)).toHaveAttribute('aria-expanded', 'false');
});

test('neutral text and control boundaries meet the documented contrast targets', async ({ page }) => {
  await ready(page);
  for (const appearance of ['浅色', '深色']) {
    await page.getByRole('button', { name: appearance, exact: true }).click();
    const contrasts = await page.evaluate(() => {
      const sample = document.createElement('span');
      // The reduced-motion baseline still has a 0.01ms transition; sample final colors synchronously.
      sample.style.setProperty('transition', 'none', 'important');
      document.body.append(sample);
      const rgb = (token: string) => {
        sample.style.color = `hsl(var(--${token}))`;
        return getComputedStyle(sample).color.match(/[\d.]+/g)!.slice(0, 3).map(Number);
      };
      const luminance = (token: string) => rgb(token).map(n => n / 255).map(n => n <= .04045 ? n / 12.92 : ((n + .055) / 1.055) ** 2.4).reduce((sum, n, i) => sum + n * [.2126, .7152, .0722][i], 0);
      const ratio = (a: string, b: string) => { const x = luminance(a), y = luminance(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05); };
      const result = { body: ratio('foreground', 'background'), muted: ratio('muted-foreground', 'muted'), primary: ratio('primary-foreground', 'primary'), input: ratio('input', 'background'), focus: ratio('ring', 'background') };
      sample.remove(); return result;
    });
    expect(contrasts.body).toBeGreaterThanOrEqual(4.5);
    expect(contrasts.muted).toBeGreaterThanOrEqual(4.5);
    expect(contrasts.primary).toBeGreaterThanOrEqual(4.5);
    expect(contrasts.input).toBeGreaterThanOrEqual(3);
    expect(contrasts.focus).toBeGreaterThanOrEqual(3);
  }
});
