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
    Object.assign(window, { runtime: { WindowMinimise: () => {}, WindowToggleMaximise: () => {}, WindowIsMaximised: async () => false, Quit: () => {} } });
  }, fixture);
  await page.goto('/');
  await page.getByRole('button', { name: 'Agent 托管', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).toBeVisible();
});

async function openSettings(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: 'Codex 托管设置', exact: true }).click();
}

test('compact platform list, inline enrollment, sync and conditional disable', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
  await expect(page.getByRole('switch')).toHaveCount(5);
  await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toHaveCount(0);
  await expect(page.getByText(fixture.workspace.targets[0].config_root, { exact: true })).toHaveCount(0);
  for (const name of ['Claude Code', 'Gemini CLI', 'OpenCode', 'DSH']) await expect(page.getByRole('switch', { name: `${name} 托管`, exact: true })).toBeDisabled();
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).not.toBeChecked();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await expect(page.getByRole('checkbox')).toHaveCount(0);
  await expect(page.getByText('使用已有模型映射', { exact: true })).toHaveCount(0);
  await expect(page.getByText('已登记的托管目标', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).toBeChecked();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([fixture.enroll]);
  await openSettings(page);
  await expect(page.getByText('配置已应用', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '同步模型目录' }).click();
  await expect.poll(() => page.evaluate(() => window.agentTest.actions)).toEqual([fixture.sync]);
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  const dialog = page.getByRole('alertdialog'); await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).toBeChecked();
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  await dialog.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).not.toBeChecked();
  expect(await page.evaluate(() => window.agentTest.actions[1])).toEqual({ ...fixture.sync, action: 'disable' });
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).toBeChecked();
  expect(await page.evaluate(() => window.agentTest.actions[2])).toEqual({ ...fixture.sync, expected_revision: 2, action: 'enable' });
  expect(errors).toEqual([]);
});

test('Agent write conflict rereads state and does not replay the write', async ({ page }) => {
  const reads = await page.evaluate(() => { window.agentTest.failWrite = true; return window.agentTest.reads; });
  await openSettings(page);
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('已重新读取');
  expect(await page.evaluate(() => window.agentTest.enrolls.length)).toBe(1);
  expect(await page.evaluate(() => window.agentTest.reads)).toBe(reads + 1);
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).not.toBeChecked();
});

test('shared mapping metadata works without repeated selection', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.models[0].upstream_models = ['upstream-with-shared-metadata']; });
  await page.getByRole('button', { name: '重新识别' }).click();
  await openSettings(page);
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([fixture.enroll]);
});

test('unavailable mapping is excluded from defaults without a redundant model list', async ({ page }) => {
  await page.evaluate(() => {
    window.agentTest.data.models.push({ ...window.agentTest.data.models[0], id: 'unavailable', available_targets: [] });
    window.agentTest.data.bindings = [{ ...window.agentTest.data.bindings[0],
      id: 'codex-fixture', config_root: 'C:\\Users\\Fixture\\.codex', model: 'unavailable', enabled: true,
      revision: 1, model_scope: 'all-compatible', catalog_version: '0.159.3', status: 'applied',
      available_models: [], catalog_path: '', model_templates: { public: '' },
    }];
  });
  await page.getByRole('button', { name: '重新识别' }).click();
  await openSettings(page);
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await expect(page.getByLabel('默认模型', { exact: true }).getByRole('option', { name: 'unavailable', exact: true })).toHaveCount(0);
  await expect(page.getByText('暂不可用：', { exact: false })).toHaveCount(0);
  await page.getByRole('button', { name: '保存并应用', exact: true }).click();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([{ ...fixture.enroll, expected_revision: 1 }]);
});

test('no compatible mappings gives a reason and blocks enrollment', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.models[0].available_targets = []; });
  await page.getByRole('button', { name: '重新识别' }).click();
  await openSettings(page);
  await expect(page.getByText('现有模型映射中没有可供此 Codex 使用的模型。', { exact: false })).toBeVisible();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('');
  await expect(page.getByRole('button', { name: '开启托管', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([]);
});

test('DSH discovery details and all brand logos stay inside platform rows', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.targets.push({ id: 'dsh-fixture', agent: 'dsh', config_root: '/isolated/.dsh',
    source: 'home', supported: false, status: 'unsupported', catalog_version: '', templates: [] }); });
  await page.getByRole('button', { name: '重新识别' }).click();
  expect(await page.locator('.agent-brand img').evaluateAll(images => images.every(image => (image as HTMLImageElement).complete && (image as HTMLImageElement).naturalWidth > 0))).toBe(true);
  await page.getByRole('button', { name: 'DSH 托管设置', exact: true }).click();
  const region = page.getByRole('region', { name: 'DSH 托管设置' });
  await expect(region).toContainText('/isolated/.dsh');
  await expect(region).toContainText('尚无适配器');
  await expect(page.getByRole('switch', { name: 'DSH 托管', exact: true })).toBeDisabled();
  await openSettings(page);
  await expect(region).toHaveCount(0);
});

test('compact layout stays within narrow windows and dark theme', async ({ page }) => {
  await page.screenshot({ path: process.env.ANCHOR_AGENT_PREVIEW ?? 'test-results/agents-collapsed.png' });
  await openSettings(page);
  await page.screenshot({ path: process.env.ANCHOR_AGENT_DETAILS_PREVIEW ?? 'test-results/agents-expanded.png' });
  await page.getByRole('button', { name: '深色', exact: true }).click();
  await expect(page.locator('html')).toHaveClass(/dark/);
  await expect(page.locator('.agent-platform-name').first()).toHaveCSS('color', 'rgb(250, 250, 250)');
  await expect(page.getByRole('heading', { name: 'Agent 托管', exact: true })).toHaveCSS('color', 'rgb(250, 250, 250)');
  await page.setViewportSize({ width: 760, height: 900 });
  const workspace = page.locator('.agent-workspace');
  expect(await workspace.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  await expect(page.getByRole('switch', { name: 'Codex 托管', exact: true })).toBeVisible();
  await page.screenshot({ path: process.env.ANCHOR_AGENT_DARK_PREVIEW ?? 'test-results/agents-dark.png' });
});
