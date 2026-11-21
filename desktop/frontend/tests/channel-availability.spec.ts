import { expect, test, type Page } from '@playwright/test';

const providerSwitch = (page: Page, id: string) => page.getByRole('switch', { name: `供应商 ${id} 启用状态`, exact: true });
async function setProvider(page: Page, id: string, enabled: boolean) {
  const toggle = providerSwitch(page, id);
  await expect(toggle).toBeEnabled();
  if (await toggle.isChecked() === enabled) return;
  await toggle.click();
  await expect(toggle).toBeChecked({ checked: enabled });
}
async function mappings(page: Page) {
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 coding-main', exact: true, includeHidden: true });
  await expect(card.locator('.mapping-candidate')).toHaveCount(3);
  await expect(card).toBeVisible();
  return card;
}

test('provider switches expose unavailable mappings without deleting them', async ({ page }) => {
  await page.goto('/');
  const toggle = providerSwitch(page, '摸鱼AI');
  await expect(toggle).toBeChecked();
  await expect(page.getByRole('heading', { name: '供应商', exact: true })).toBeVisible();
  await setProvider(page, '摸鱼AI', false);
  await setProvider(page, '备用供应商', false);
  const card = await mappings(page);
  await expect(card).toContainText('无有效渠道');
  await expect(card).toHaveClass(/availability-unavailable/);
  await expect(card.locator('.channel-status[title^="供应商已停用"]')).toHaveCount(3);
  await expect(card.locator('.mapping-key-status').first()).toContainText('Key 已启用');
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await setProvider(page, '摸鱼AI', true);
  const restored = await mappings(page);
  await expect(restored).toContainText('1/2 渠道可用');
  await expect(restored.locator('.model-unavailable-notice')).toHaveCount(0);
});

test('Key switch preserves its setting independently and disabled Keys remain visible in mappings', async ({ page }) => {
  await page.goto('/');
  await setProvider(page, '备用供应商', false);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  const detail = page.getByLabel('摸鱼AI 的详情');
  await detail.getByRole('tab', { name: /^Key/ }).click();
  const key = detail.getByRole('switch', { name: 'Key default 启用状态', exact: true });
  await expect(key).toBeChecked();
  await key.click();
  await expect(key).not.toBeChecked();
  await expect(key).toBeEnabled();
  let card = await mappings(page);
  await expect(card).toContainText('无有效渠道');
  const source = card.locator('.mapping-candidate').first();
  await expect(source.locator('.channel-status').first()).toHaveAttribute('title', /供应商已启用/);
  await expect(source.locator('.channel-status').first()).toHaveText('已停用');
  await expect(source.locator('.mapping-key-status')).toContainText('Key 已停用');
  await expect(source).not.toContainText('匿名来源');
  await source.getByRole('button', { name: /摸鱼AI.*编辑映射/ }).click();
  await detail.getByRole('tab', { name: /^Key/ }).click();
  await expect(key).not.toBeChecked();
  await key.click();
  await expect(key).toBeChecked();
  await expect(key).toBeEnabled();
  card = await mappings(page);
  await expect(card).toContainText('1/2 渠道可用');
});

test('unavailable sources and models are shorter, neutral and still operable', async ({ page }) => {
  await page.goto('/');
  await setProvider(page, '备用供应商', false);
  const card = await mappings(page);
  const live = card.locator('.mapping-key-card.availability-available').first();
  const inactive = card.locator('.mapping-key-card.availability-unavailable').first();
  const liveHeight = (await live.boundingBox())!.height;
  const fullHeight = (await card.boundingBox())!.height;
  await expect(inactive.locator('.mapping-key-pricing')).toHaveCount(0);
  await expect(live.locator('.mapping-key-pricing')).toBeVisible();
  expect((await inactive.boundingBox())!.height).toBeLessThan(liveHeight * 0.8);
  if (process.env.ANCHOR_MAPPING_PREVIEW) await card.screenshot({ path: `${process.env.ANCHOR_MAPPING_PREVIEW}/mixed-availability.png` });
  // Compact cards remain in persisted order, and keyboard reordering still saves.
  const grip = inactive.getByRole('button', { name: /^拖动/ });
  await grip.press('ArrowUp');
  await expect(card).toContainText('已自动保存');
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await setProvider(page, '摸鱼AI', false);
  await setProvider(page, '备用供应商', false);
  await mappings(page);
  await expect(card).toHaveClass(/availability-unavailable/);
  expect((await card.boundingBox())!.height).toBeLessThan(fullHeight);
  await expect(card.locator('.mapping-key-card.is-compact')).toHaveCount(3);
  await expect(card.getByLabel('调度规则 coding-main')).toBeEnabled();
  await expect(card.getByRole('button', { name: '模型设置 coding-main', exact: true })).toBeEnabled();
  for (const theme of ['浅色', '深色'] as const) {
    await page.getByRole('button', { name: theme, exact: true }).click();
    const colors = await card.evaluate(el => {
      const source = el.querySelector('.mapping-key-card')!;
      return { background: getComputedStyle(el).backgroundImage, source: getComputedStyle(source).backgroundColor, status: getComputedStyle(source.querySelector('.channel-status')!).backgroundColor };
    });
    expect(colors.background).toBe('none');
    expect(colors.source).toMatch(/rgba?\((\d+), \1, \1/);
    expect(colors.status).toMatch(/rgba?\((\d+), \1, \1/);
    if (process.env.ANCHOR_MAPPING_PREVIEW) await card.screenshot({ path: `${process.env.ANCHOR_MAPPING_PREVIEW}/unavailable-${theme === '浅色' ? 'light' : 'dark'}.png` });
  }
  await page.setViewportSize({ width: 600, height: 850 });
  await expect(card.getByLabel('调度规则 coding-main')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await card.locator('.candidate').first().focus();
  await expect(card.locator('.candidate').first()).toBeFocused();
});
