import { expect, test } from '@playwright/test';

test('quick test keeps the provider list visible and displays incremental output', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '测试 摸鱼AI', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(page.getByLabel('摸鱼AI 的详情')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '返回供应商列表', exact: true })).toHaveCount(0);
  await expect(dialog.getByLabel('使用 Key')).toHaveValue('default');
  await expect(dialog.getByLabel('可用模型')).toHaveValue('demo-model');
  const labels = await dialog.locator('label').allTextContents();
  expect(labels.indexOf('使用 Key')).toBeLessThan(labels.indexOf('可用模型'));
  await dialog.getByRole('button', { name: '发起测试', exact: true }).click();
  await expect(dialog.getByLabel('模型返回内容')).toHaveText('O');
  await expect(dialog.getByRole('button', { name: '停止测试', exact: true })).toBeVisible();
  await expect(dialog.getByLabel('模型返回内容')).toHaveText('OK');
  await expect(dialog.getByText('推理测试成功', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
});

test('switching Key reloads available models and a running test can be stopped', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '测试 备用供应商', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('可用模型')).toHaveValue('demo-model');
  await dialog.getByLabel('使用 Key').selectOption('primary');
  await expect(dialog.getByLabel('可用模型')).toBeDisabled();
  await expect(dialog.getByRole('button', { name: '发起测试', exact: true })).toBeEnabled();
  await dialog.getByRole('button', { name: '发起测试', exact: true }).click();
  await expect(dialog.getByLabel('使用 Key')).toBeDisabled();
  await dialog.getByRole('button', { name: '停止测试', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('测试已停止');
  await expect(dialog.getByLabel('使用 Key')).toBeEnabled();
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByLabel('备用供应商 的详情')).toHaveCount(0);
});

test('detail test stays on the detail page after closing', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByLabel('摸鱼AI 的详情').getByRole('button', { name: '测试连通', exact: true }).first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('可用模型')).toHaveValue('demo-model');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByLabel('摸鱼AI 的详情')).toBeVisible();
});

test('model row test uses the saved upstream model without Key or catalog selection', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await page.getByRole('button', { name: '测试模型 coding-main', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.locator('select')).toHaveCount(0);
  await expect(dialog.getByText('demo-model-mini', { exact: true })).toBeVisible();
  await expect(dialog.getByText('default', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '发起测试', exact: true })).toBeEnabled();
  // The synthetic catalog offers demo-model, not demo-model-mini: no discovery fallback is allowed.
  await expect(dialog.getByLabel('模型返回内容')).toHaveCount(0);
  await dialog.getByRole('button', { name: '发起测试', exact: true }).click();
  await expect(dialog.getByText('推理测试成功', { exact: true })).toBeVisible();
  await expect(dialog.locator('.probe-result').filter({ hasText: 'Key default' }).filter({ hasText: 'demo-model-mini' })).toBeVisible();
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByRole('button', { name: '测试模型 coding-main', exact: true })).toBeVisible();
});

test('model row test respects the saved Key binding instead of the first provider Key', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '备用供应商', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await page.getByRole('button', { name: '编辑模型 coding-main', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox', { name: 'primary', exact: true }).check();
  await dialog.getByRole('button', { name: '保存映射', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: '测试模型 coding-main', exact: true }).click();
  await expect(dialog.locator('select')).toHaveCount(0);
  await expect(dialog.getByText('primary', { exact: true })).toBeVisible();
  await expect(dialog.getByText('backup-model', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '发起测试', exact: true }).click();
  await expect(dialog.getByText('推理测试成功', { exact: true })).toBeVisible();
  await expect(dialog.locator('.probe-result').filter({ hasText: 'Key primary' }).filter({ hasText: 'backup-model' })).toBeVisible();
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('tab', { name: /^Key/ }).click();
  await page.getByRole('switch', { name: 'Key primary 启用状态', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Key primary 启用状态', exact: true })).not.toBeChecked();
  await expect(page.getByRole('switch', { name: 'Key primary 启用状态', exact: true })).toBeEnabled();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await page.getByRole('button', { name: '测试模型 coding-main', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('此模型映射绑定的 Key 均未启用或已不存在');
  await expect(dialog.getByRole('button', { name: '发起测试', exact: true })).toBeDisabled();
});
