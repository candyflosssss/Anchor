import { expect, test } from '@playwright/test';
import fixture from '../../contract/agents.json' with { type: 'json' };
import type { AgentWorkspace, DesktopBridge, EnrollAgent, ManageAgent } from '../src/desktop/contract.generated';

declare global { interface Window { agentTest: { data: AgentWorkspace; reads: number; enrolls: EnrollAgent[]; actions: ManageAgent[]; failWrite: boolean } } }
test.beforeEach(async ({ page }) => {
  await page.addInitScript(f => {
    const data: AgentWorkspace = { ...f.workspace, bindings: [] };
    window.agentTest = { data, reads: 0, enrolls: [], actions: [], failWrite: false };
    const result = <T,>(value: T) => ({ contract: f.contract, data: value, problem: null });
    const session = { instance_id: 'agent-isolated', phase: 'ready', source: 'started', can_stop: true };
    const api = {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      AgentWorkspace: async () => { window.agentTest.reads++; return result(data); },
      EnrollAgent: async (input: EnrollAgent) => {
        window.agentTest.enrolls.push(input);
        if (window.agentTest.failWrite) return { contract: f.contract, data: null, problem: { code: 'conflict' } };
        data.bindings = [{ ...f.workspace.bindings[0], model: input.model, model_templates: input.model_templates }];
        return result(data);
      },
      ManageAgent: async (input: ManageAgent) => {
        window.agentTest.actions.push(input);
        const row = data.bindings[0];
        if (input.action === 'disable') { row.enabled = false; row.status = 'restored'; row.revision++; }
        if (input.action === 'enable') { row.enabled = true; row.status = 'applied'; row.revision++; }
        return result(data);
      },
    } as unknown as DesktopBridge;
    window.go = { desktop: { Bridge: api } };
  }, fixture);
  await page.goto('/');
  await page.getByRole('button', { name: 'Agent 托管', exact: true }).click();
  await expect(page.getByRole('button', { name: '设置托管', exact: true })).toBeVisible();
});

test('Agent discovery, approved enrollment, sync and conditional disable', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
  await page.getByRole('button', { name: '设置托管', exact: true }).click();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await expect(page.getByRole('list', { name: '将提供给 Codex 的模型' })).toContainText('public');
  await expect(page.getByText('模型目录模板', { exact: true })).toHaveCount(0);
  await expect(page.locator('select')).toHaveCount(1);
  const save = page.getByRole('button', { name: '保存并应用托管' });
  await expect(save).toBeDisabled();
  await page.getByRole('checkbox').check(); await save.click();
  await expect(page.getByText('配置已应用', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([fixture.enroll]);
  await page.getByRole('button', { name: '同步模型目录' }).click();
  await expect.poll(() => page.evaluate(() => window.agentTest.actions)).toEqual([fixture.sync]);
  await page.getByRole('button', { name: '停用并恢复配置' }).click();
  const dialog = page.getByRole('alertdialog'); await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByText('已停用', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.agentTest.actions[1])).toEqual({ ...fixture.sync, action: 'disable' });
  expect(errors).toEqual([]);
});

test('Agent write conflict rereads state and does not replay the write', async ({ page }) => {
  const reads = await page.evaluate(() => { window.agentTest.failWrite = true; return window.agentTest.reads; });
  await page.getByRole('button', { name: '设置托管', exact: true }).click();
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: '保存并应用托管' }).click();
  await expect(page.getByRole('alert')).toContainText('已重新读取');
  expect(await page.evaluate(() => window.agentTest.enrolls.length)).toBe(1);
  expect(await page.evaluate(() => window.agentTest.reads)).toBe(reads + 1);
  await expect(page.getByText('还没有托管目标。', { exact: true })).toBeVisible();
});


test('shared mapping metadata works without a matching local template or repeated selection', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.models[0].upstream_models = ['upstream-with-shared-metadata']; });
  await page.getByRole('button', { name: '重新识别' }).click();
  await page.getByRole('button', { name: '设置托管', exact: true }).click();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: '保存并应用托管' }).click();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([fixture.enroll]);
});

test('unavailable mapping is explained and cannot become the default', async ({ page }) => {
  await page.evaluate(() => {
    window.agentTest.data.models.push({ ...window.agentTest.data.models[0], id: 'unavailable', available_targets: [] });
    window.agentTest.data.bindings = [{
      id: 'codex-fixture', config_root: 'C:\\Users\\Fixture\\.codex', model: 'unavailable', enabled: true,
      revision: 1, model_scope: 'all-compatible', catalog_version: '0.159.3', status: 'applied',
      available_models: [], catalog_path: '', model_templates: { public: '' },
    }];
  });
  await page.getByRole('button', { name: '重新识别' }).click();
  await page.getByRole('button', { name: '设置托管', exact: true }).click();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await expect(page.getByLabel('默认模型', { exact: true }).getByRole('option', { name: 'unavailable', exact: true })).toHaveCount(0);
  await expect(page.getByText('暂不可用：unavailable。', { exact: false })).toBeVisible();
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: '保存并应用托管' }).click();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([{ ...fixture.enroll, expected_revision: 1 }]);
});

test('no compatible mappings gives a reason and blocks enrollment', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.models[0].available_targets = []; });
  await page.getByRole('button', { name: '重新识别' }).click();
  await page.getByRole('button', { name: '设置托管', exact: true }).click();
  await expect(page.getByText('现有模型映射中没有可供此 Codex 使用的模型。', { exact: false })).toBeVisible();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('');
  await page.getByRole('checkbox').check();
  await expect(page.getByRole('button', { name: '保存并应用托管' })).toBeDisabled();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([]);
});
