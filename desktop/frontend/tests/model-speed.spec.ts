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

test('mapping settings directly creates speed siblings without navigating to providers', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 coding-main', exact: true });
  await card.getByRole('button', { name: '模型设置 coding-main', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '速度与分身', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const source = dialog.getByLabel('分身来源渠道', { exact: true });
  // A shared model requires an explicit source, not an arbitrary first channel.
  await expect(source).toHaveValue('');
  await source.selectOption('摸鱼AI');
  await expect(dialog.getByLabel('Anchor 固定速度档位')).toHaveValue('');
  await dialog.getByRole('checkbox', { name: 'Fast · coding-main(fast)', exact: true }).check();
  await dialog.getByRole('checkbox', { name: 'Ultrafast · coding-main(ultrafast)', exact: true }).check();
  if (process.env.ANCHOR_SPEED_PREVIEW) await dialog.screenshot({ path: process.env.ANCHOR_SPEED_PREVIEW });
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('速度设置已保存');
  await expect(dialog.getByRole('checkbox', { name: 'Fast · coding-main(fast)', exact: true })).not.toBeChecked();
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByRole('heading', { name: '模型映射', exact: true })).toBeVisible();
  for (const speed of ['fast', 'ultrafast']) {
    const sibling = page.getByRole('article', { name: `模型 coding-main(${speed})`, exact: true });
    await expect(sibling).toContainText('1/1 可用');
    await sibling.getByRole('button', { name: `模型设置 coding-main(${speed})`, exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: '速度与分身', exact: true }).click();
    await expect(dialog.getByLabel('Anchor 固定速度档位')).toHaveValue(speed === 'fast' ? 'priority' : 'ultrafast');
    await expect(dialog.getByRole('checkbox')).toHaveCount(0);
    await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  }
  await card.getByRole('button', { name: '模型设置 coding-main', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '速度与分身', exact: true }).click();
  await source.selectOption('摸鱼AI');
  await expect(dialog.getByLabel('Anchor 固定速度档位')).toHaveValue('');
  await dialog.getByRole('checkbox', { name: 'Fast · coding-main(fast)', exact: true }).check();
  await expect(dialog.getByRole('alert')).toContainText('已有同名速度分身');
  await expect(dialog.getByRole('button', { name: '保存速度设置', exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '常规设置', exact: true }).click();
  await expect(dialog.getByRole('button', { name: '常规设置', exact: true })).toHaveAttribute('aria-current', 'page');
  await dialog.getByRole('button', { name: '速度与分身', exact: true }).click();
  await expect(source).toHaveValue('');
});

test('mapping settings changes and clears a shared model tier without creating siblings', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 coding-main', exact: true });
  await card.getByRole('button', { name: '模型设置 coding-main', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '速度与分身', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('分身来源渠道').selectOption('摸鱼AI');
  const tier = dialog.getByLabel('Anchor 固定速度档位');
  await expect(tier).toBeEnabled();
  await tier.selectOption('priority');
  await expect(dialog.getByRole('checkbox')).toHaveCount(0);
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('速度设置已保存');
  await dialog.getByLabel('分身来源渠道').selectOption('备用供应商');
  await expect(tier).toHaveValue('priority');
  await expect(tier).toBeEnabled();
  await tier.selectOption('');
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('速度设置已保存');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(card).not.toContainText('Anchor 固定档位');
  await expect(page.getByRole('article', { name: '模型 coding-main(fast)', exact: true })).toHaveCount(0);
});
