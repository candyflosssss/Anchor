import { expect, test } from '@playwright/test';

async function mappings(page: import('@playwright/test').Page) {
  await page.goto('/');
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 coding-main', exact: true });
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled();
  return card;
}

test('manual order, quick sorts, cancellation and persistence', async ({ page }) => {
  const card = await mappings(page);
  const rows = card.locator('.mapping-candidate');
  await expect(rows).toHaveCount(2);
  await card.getByRole('button', { name: '下移 摸鱼AI demo-model-mini', exact: true }).click();
  await expect(rows.first()).toContainText('备用供应商');
  await expect(card.getByLabel('调度规则 coding-main')).toHaveValue('priority');
  await expect(card).toContainText('已自动保存');
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled();
  // Equal unknown rates retain the acknowledged manual order.
  await card.getByRole('button', { name: '按倍率', exact: true }).click();
  await expect(rows.first()).toContainText('备用供应商');
  await card.getByRole('button', { name: '低延迟优先', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('可能产生费用');
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  await expect(card.getByRole('button', { name: '停止', exact: true })).toHaveCount(0);
  await card.getByRole('button', { name: '低延迟优先', exact: true }).click();
  await confirm.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(card.getByRole('button', { name: '停止', exact: true })).toBeVisible();
  await card.getByRole('button', { name: '停止', exact: true }).click();
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled();
  await expect(rows.first()).toContainText('备用供应商');
  await card.getByRole('button', { name: '低延迟优先', exact: true }).click();
  await confirm.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled({ timeout: 15000 });
  await expect(page.getByRole('alert')).toHaveCount(0);
  // Keyboard dragging uses the same save path as pointer dragging.
  await card.getByRole('button', { name: /^拖动/ }).first().press('ArrowDown');
  await expect(card).toContainText('已自动保存');
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled();
  const first = await rows.first().locator('.candidate strong').first().innerText();
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  await expect(rows.first()).toContainText(first);
});

test('pointer dragging and compact layouts in light and dark themes', async ({ page }) => {
  const card = await mappings(page);
  const handle = card.getByRole('button', { name: '拖动 摸鱼AI demo-model-mini', exact: true });
  const from = await handle.boundingBox(), to = await card.locator('.mapping-key-card').last().boundingBox();
  if (!from || !to) throw Error('missing route geometry');
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 8 });
  await page.mouse.up();
  await expect(card.locator('.mapping-candidate').first()).toContainText('备用供应商');
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeEnabled();
  if (process.env.ANCHOR_MAPPING_PREVIEW) await card.screenshot({ path: `${process.env.ANCHOR_MAPPING_PREVIEW}/mapping-light.png` });
  await page.getByRole('button', { name: '深色', exact: true }).click();
  if (process.env.ANCHOR_MAPPING_PREVIEW) await card.screenshot({ path: `${process.env.ANCHOR_MAPPING_PREVIEW}/mapping-dark.png` });
  await page.setViewportSize({ width: 600, height: 850 });
  await expect(card.getByRole('button', { name: '按倍率', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (process.env.ANCHOR_MAPPING_PREVIEW) await page.screenshot({ path: `${process.env.ANCHOR_MAPPING_PREVIEW}/mapping-narrow.png` });
});
