import { expect, test } from '@playwright/test';

test('creates Fast and Ultrafast siblings and preserves their explicit policy on edit', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  const catalog = page.getByRole('region', { name: '上游模型目录', exact: true });
  await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await catalog.getByRole('button', { name: '导入 demo-model', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Anchor 公开别名', { exact: true }).fill('gpt-6.1-sol');
  await dialog.getByRole('checkbox', { name: 'Fast · gpt-6.1-sol(fast)', exact: true }).check();
  await dialog.getByRole('checkbox', { name: 'Ultrafast · gpt-6.1-sol(ultrafast)', exact: true }).check();
  await expect(dialog).toContainText('需要上游支持，可能增加费用');
  await dialog.getByRole('button', { name: '添加映射', exact: true }).click();
  await expect(dialog).toBeHidden();
  for (const [speed, tier] of [['fast', 'priority'], ['ultrafast', 'ultrafast']]) {
    await page.getByRole('button', { name: `编辑模型 gpt-6.1-sol(${speed})`, exact: true }).click();
    await expect(dialog.getByLabel('Anchor 固定速度档位', { exact: true })).toHaveValue(tier);
    await expect(dialog.getByLabel('上游模型 ID', { exact: true })).toHaveValue('demo-model');
    await expect(dialog.getByLabel('函数工具', { exact: true })).toBeChecked();
    await expect(dialog.getByLabel('流式输出', { exact: true })).toBeChecked();
    await dialog.getByRole('button', { name: '保存映射', exact: true }).click();
    await expect(dialog).toBeHidden();
  }
});

test('can add a single sibling to an existing ordinary mapping and blocks duplicate selection', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  const catalog = page.getByRole('region', { name: '上游模型目录', exact: true });
  await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await catalog.getByRole('button', { name: '快速导入 demo-model', exact: true }).click();
  await expect(catalog.getByRole('status')).toContainText('已导入 1 / 1 个');
  await page.getByRole('button', { name: '编辑模型 demo-model', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('checkbox', { name: 'Fast · demo-model(fast)', exact: true }).check();
  await dialog.getByRole('button', { name: '保存映射', exact: true }).click();
  await expect(dialog).toBeHidden();
  await page.getByRole('button', { name: '编辑模型 demo-model', exact: true }).click();
  await dialog.getByRole('checkbox', { name: 'Fast · demo-model(fast)', exact: true }).check();
  await expect(dialog.getByRole('alert')).toContainText('已有同名速度分身');
  await expect(dialog.getByRole('button', { name: '保存映射', exact: true })).toBeDisabled();
  await dialog.getByLabel('Anchor 固定速度档位', { exact: true }).selectOption('priority');
  await expect(dialog.getByRole('checkbox', { name: 'Fast · demo-model(fast)', exact: true })).toHaveCount(0);
  await expect(dialog.getByRole('alert')).toHaveCount(0);
});
