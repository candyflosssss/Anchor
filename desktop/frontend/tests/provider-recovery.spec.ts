import { expect, test, type Page } from '@playwright/test';

async function openKeys(page: Page, scenario: 'conflict' | 'unknown', id = '摸鱼AI') {
  await page.goto('/');
  await expect(page.getByRole('button', { name: id, exact: true })).toBeVisible();
  await page.getByLabel('原型场景').selectOption(scenario);
  await expect(page.getByText('正在读取供应商', { exact: true })).toBeHidden();
  await page.getByRole('button', { name: id, exact: true }).click();
  await page.getByRole('tab', { name: /^Key/ }).click();
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
}

test('a Key conflict automatically syncs even when the provider revision is unchanged', async ({ page }) => {
  await openKeys(page, 'conflict');
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByRole('status').filter({ hasText: '已同步最新配置' })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
  // The failed write is never replayed by a read.
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('round-robin');
  await page.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(page.getByText('revision 3', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: /^Key/ }).click();
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('priority');
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(page.getByText('revision 4', { exact: true })).toBeVisible();
});

test('reread resolves an unknown Key reset without a provider revision bump', async ({ page }) => {
  await openKeys(page, 'unknown', '备用供应商');
  await page.getByRole('button', { name: '恢复 Key primary', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('写入结果未知');
  await page.getByRole('button', { name: '重新读取', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
  await expect(page.getByRole('button', { name: '恢复 Key primary', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '停用 Key primary', exact: true }).click();
  await expect(page.getByRole('button', { name: '启用 Key primary', exact: true })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('reread checks an unknown saved strategy and the next write uses the latest revision', async ({ page }) => {
  await openKeys(page, 'unknown');
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByRole('alert')).toContainText('写入结果未知');
  await page.getByRole('button', { name: '重新读取', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('priority');
  await page.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(page.getByText('revision 4', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: /^Key/ }).click();
  await page.getByLabel('Key 调度策略').selectOption('round-robin');
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('round-robin');
  await page.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(page.getByText('revision 5', { exact: true })).toBeVisible();
});

test('a failed detail reread retains the write lock and another reread can recover', async ({ page }) => {
  await openKeys(page, 'conflict');
  // Fail only the next detail read. List reads still succeed at the same revision.
  await page.evaluate(async () => {
    const modulePath = '/src/features/providers/provider.mock.ts';
    const { MockProviderTransport } = await import(modulePath) as typeof import('../src/features/providers/provider.mock');
    const workspace = MockProviderTransport.prototype.workspace;
    MockProviderTransport.prototype.workspace = async function () {
      MockProviderTransport.prototype.workspace = workspace;
      await new Promise(resolve => setTimeout(resolve, 500));
      throw new Error('synthetic detail read failure');
    };
  });
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByLabel('Key 调度策略')).toBeDisabled();
  await expect(page.getByRole('alert')).toContainText('读取详情失败');
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('round-robin');
  await expect(page.getByRole('alert')).toContainText('读取详情失败');
  await page.getByRole('button', { name: '重新读取', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
  await page.getByLabel('Key 调度策略').selectOption('priority');
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('priority');
  await page.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(page.getByText('revision 4', { exact: true })).toBeVisible();
});

test('a new provider automatically reconciles concurrent changes before its next action', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
  // Simulate a concurrent write after creation, before the save response is consumed.
  await page.evaluate(async () => {
    const mockPath = '/src/features/providers/provider.mock.ts';
    const actionPath = '/src/features/providers/provider-features.model.ts';
    const { MockProviderTransport } = await import(mockPath) as typeof import('../src/features/providers/provider.mock');
    const { actionFor } = await import(actionPath) as typeof import('../src/features/providers/provider-features.model');
    const save = MockProviderTransport.prototype.save;
    MockProviderTransport.prototype.save = async function (input, signal) {
      MockProviderTransport.prototype.save = save;
      const saved = await save.call(this, input, signal);
      await this.action(actionFor(saved, 'strategy', { strategy: 'priority' }), signal);
      return saved;
    };
  });
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  await page.getByLabel('供应商名称 / ID').fill('新建供应商');
  await page.getByLabel('API 根地址', { exact: true }).fill('https://new.example.com/v1');
  await page.getByLabel('API 密钥（可选）').fill('synthetic-new-provider-key');
  await page.getByRole('dialog').getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeHidden();
  const detail = page.getByLabel('新建供应商 的详情');
  await expect(detail).toContainText('revision 2');
  await detail.getByRole('tab', { name: /^Key/ }).click();
  await expect(page.getByLabel('Key 调度策略')).toBeEnabled();
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('priority');
  await page.getByLabel('Key 调度策略').selectOption('round-robin');
  await expect(page.getByLabel('Key 调度策略')).toHaveValue('round-robin');
  await expect(page.getByRole('alert')).toHaveCount(0);
  await detail.getByRole('tab', { name: '概况', exact: true }).click();
  await expect(detail).toContainText('revision 3');
});

test('a Key dialog shows automatic conflict recovery and waits for a deliberate resubmit', async ({ page }) => {
  await openKeys(page, 'conflict');
  await page.getByRole('button', { name: '添加 Key', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await page.getByLabel('Key 名称', { exact: true }).fill('second');
  await page.getByLabel('API Key', { exact: true }).fill('synthetic-conflict-key');
  await dialog.getByRole('button', { name: '保存 Key', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('已同步最新配置');
  await expect(dialog.getByRole('status')).toContainText('刚才的操作未保存');
  await expect(page.getByLabel('API Key', { exact: true })).toHaveValue('');
  await expect(dialog.getByRole('button', { name: '保存 Key', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: '编辑 Key second', exact: true })).toHaveCount(0);
  await page.getByLabel('API Key', { exact: true }).fill('synthetic-conflict-key');
  await dialog.getByRole('button', { name: '保存 Key', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('button', { name: '编辑 Key second', exact: true })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
});
