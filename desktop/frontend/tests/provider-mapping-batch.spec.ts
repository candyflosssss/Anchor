import { expect, test, type Page } from '@playwright/test';

async function mappings(page: Page) {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await expect(page.getByRole('checkbox', { name: '选择模型映射 coding-main', exact: true })).toBeEnabled();
}
async function addMapping(page: Page) {
  await page.getByRole('button', { name: '手动添加映射', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('上游模型 ID').fill('vendor/second');
  await dialog.getByLabel('Anchor 公开别名').fill('batch-second');
  await dialog.getByRole('checkbox', { name: '流式输出', exact: true }).uncheck();
  await dialog.getByRole('checkbox', { name: 'default', exact: true }).check();
  await dialog.getByLabel('Anchor 固定速度档位').selectOption('priority');
  await dialog.getByRole('button', { name: '添加映射', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('checkbox', { name: '全选模型映射', exact: true })).toBeEnabled();
}

test('selection and partial bulk edit preserve per-row upstreams, keys, tier and untouched capabilities', async ({ page }) => {
  await mappings(page); await addMapping(page);
  const all = page.getByRole('checkbox', { name: '全选模型映射', exact: true });
  await expect(page.getByRole('button', { name: '批量编辑', exact: true })).toBeDisabled();
  await page.getByRole('checkbox', { name: '选择模型映射 coding-main', exact: true }).check();
  await expect(all).toBeChecked({ indeterminate: true });
  await all.check();
  await expect(page.getByRole('group', { name: '模型映射批量操作' })).toContainText('已选 2 / 2');
  await page.getByRole('button', { name: '批量编辑', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('button', { name: '保存批量修改' })).toBeDisabled();
  await expect(dialog).toContainText('会影响同名模型的其他供应商渠道');
  await dialog.getByLabel('函数工具', { exact: true }).selectOption('false');
  await dialog.getByRole('button', { name: '保存批量修改' }).click();
  await expect(dialog.getByRole('status')).toContainText('已完成 · 2 / 2 · 成功 2');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('button', { name: '编辑模型 batch-second', exact: true }).click();
  await expect(dialog.getByLabel('上游模型 ID')).toHaveValue('vendor/second');
  await expect(dialog.getByRole('checkbox', { name: '函数工具', exact: true })).not.toBeChecked();
  await expect(dialog.getByRole('checkbox', { name: '流式输出', exact: true })).not.toBeChecked();
  await expect(dialog.getByRole('checkbox', { name: 'default', exact: true })).toBeChecked();
  await expect(dialog.getByLabel('Anchor 固定速度档位')).toHaveValue('priority');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await page.getByRole('button', { name: '编辑模型 coding-main', exact: true }).click();
  await expect(dialog.getByLabel('上游模型 ID')).toHaveValue('demo-model-mini');
  await expect(dialog.getByRole('checkbox', { name: '流式输出', exact: true })).toBeChecked();
  await expect(dialog.getByRole('checkbox', { name: 'default', exact: true })).not.toBeChecked();
  await expect(dialog.getByLabel('Anchor 固定速度档位')).toHaveValue('');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await page.getByRole('button', { name: '清空选择', exact: true }).click();
  await expect(page.getByRole('button', { name: '批量编辑', exact: true })).toBeDisabled();
});

test('bulk tests require cost confirmation and retain a result for every selected mapping', async ({ page }) => {
  await mappings(page); await addMapping(page);
  await page.getByRole('checkbox', { name: '全选模型映射', exact: true }).check();
  await page.getByRole('button', { name: '批量测试', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('可能产生用量');
  await expect(dialog.getByLabel('批量操作结果')).toHaveCount(0);
  await dialog.getByRole('button', { name: '确认并开始测试' }).click();
  await expect(dialog.getByRole('status')).toContainText('已完成 · 2 / 2 · 成功 2');
  const results = dialog.getByLabel('批量操作结果');
  await expect(results.getByRole('listitem').filter({ hasText: 'coding-main' })).toContainText('Key default');
  await expect(results.getByRole('listitem').filter({ hasText: 'batch-second' })).toContainText('Key default');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('button', { name: '返回供应商列表' }).click();
  await page.getByRole('button', { name: '备用供应商', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await expect(page.getByRole('group', { name: '模型映射批量操作' })).toContainText('已选 0 / 1');
});

test('bulk delete is confirmed, removes only selected provider mappings and clears deleted selection', async ({ page }) => {
  await mappings(page); await addMapping(page);
  await page.getByRole('checkbox', { name: '全选模型映射', exact: true }).check();
  await page.getByRole('button', { name: '批量删除', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('此操作不可撤销');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await expect(page.getByRole('checkbox', { name: /^选择模型映射 / })).toHaveCount(2);
  await page.getByRole('button', { name: '批量删除', exact: true }).click();
  await dialog.getByRole('button', { name: '确认删除所选映射' }).click();
  await expect(dialog.getByRole('status')).toContainText('已完成 · 2 / 2 · 成功 2');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByText('还没有映射模型。', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '返回供应商列表' }).click();
  await page.getByRole('button', { name: '备用供应商', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await expect(page.getByRole('button', { name: '编辑模型 coding-main', exact: true })).toBeVisible();
});

test('stopping a batch finishes only the in-flight item and leaves the rest unexecuted', async ({ page }) => {
  await mappings(page); await addMapping(page);
  await page.getByRole('checkbox', { name: '全选模型映射', exact: true }).check();
  await page.getByRole('button', { name: '批量测试', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: '确认并开始测试' }).click();
  await dialog.getByRole('button', { name: '停止后续操作' }).click();
  await expect(dialog.getByRole('status')).toContainText('已停止 · 1 / 2 · 成功 1');
  await expect(dialog.getByLabel('批量操作结果').getByRole('listitem').filter({ hasText: 'batch-second' })).toContainText('未执行');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
});
