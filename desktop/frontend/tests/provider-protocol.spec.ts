import { expect, test } from '@playwright/test';

test('request channel can be edited, cancelled and read back with the default Key', async ({ page }) => {
  await page.goto('/');
  const edit = page.getByRole('button', { name: '编辑 摸鱼AI', exact: true });
  await edit.click();
  let dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('请求渠道', { exact: true })).toHaveValue('responses');
  await dialog.getByLabel('请求渠道', { exact: true }).selectOption('anthropic');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await edit.click();
  dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('请求渠道', { exact: true })).toHaveValue('responses');
  await dialog.getByLabel('请求渠道', { exact: true }).selectOption('chat-completions');
  await dialog.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(dialog).toBeHidden();
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  const detail = page.getByLabel('摸鱼AI 的详情');
  await expect(detail).toContainText('Chat Completions');
  await detail.getByRole('tab', { name: /^Key/ }).click();
  await expect(detail.getByRole('row').filter({ has: page.getByRole('cell', { name: 'default', exact: true }) })).toContainText('Chat Completions');
  await detail.getByRole('button', { name: '编辑配置', exact: true }).click();
  await expect(page.getByRole('dialog').getByLabel('请求渠道', { exact: true })).toHaveValue('chat-completions');
});

test('new anonymous supplier uses its selected request channel', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('请求渠道', { exact: true })).toHaveValue('responses');
  await dialog.getByLabel('供应商名称 / ID', { exact: true }).fill('渠道测试');
  await dialog.getByLabel('API 根地址', { exact: true }).fill('https://example.invalid/v1');
  await dialog.getByLabel('请求渠道', { exact: true }).selectOption('anthropic');
  await dialog.getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByLabel('渠道测试 的详情')).toContainText('Anthropic Messages');
});
