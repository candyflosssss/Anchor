import { expect, test } from '@playwright/test';
import fixture from '../../contract/agents.json' with { type: 'json' };
import type { AgentWorkspace, DesktopBridge, EnrollAgent, ManageAgent, ProblemCode } from '../src/desktop/contract.generated';

declare global { interface Window { agentTest: { data: AgentWorkspace; reads: number; enrolls: EnrollAgent[]; actions: ManageAgent[]; failWrite: boolean; failureCode?: ProblemCode; failRead?: boolean } } }
test.beforeEach(async ({ page }) => {
  await page.addInitScript(f => {
    const data: AgentWorkspace = { ...f.workspace, bindings: [] };
    window.agentTest = { data, reads: 0, enrolls: [], actions: [], failWrite: false };
    const result = <T,>(value: T) => ({ contract: f.contract, data: value, problem: null });
    const session = { instance_id: 'agent-isolated', phase: 'ready', source: 'started', can_stop: true };
    const api = {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      AgentWorkspace: async () => { window.agentTest.reads++; return window.agentTest.failRead ? { contract: f.contract, data: null, problem: { code: 'read_error' } } : result(data); },
      EnrollAgent: async (input: EnrollAgent) => {
        window.agentTest.enrolls.push(input);
        if (window.agentTest.failWrite) return { contract: f.contract, data: null, problem: { code: window.agentTest.failureCode ?? 'conflict' } };
        const target = data.targets.find(t => t.id === input.target_id)!;
        data.bindings = [...data.bindings.filter(b => b.id !== target.id), { ...f.workspace.bindings[0], id: target.id, adapter_id: target.agent, config_root: target.config_root, model_scope: input.model_scope, model: input.model, model_templates: input.model_templates }];
        return result(data);
      },
      ManageAgent: async (input: ManageAgent) => {
        window.agentTest.actions.push(input);
        const row = data.bindings.find(b => b.id === input.binding_id)!;
        if (window.agentTest.failWrite) {
          if (input.action === 'enable') { row.enabled = true; row.revision++; row.status = 'apply_failed'; }
          return { contract: f.contract, data: null, problem: { code: window.agentTest.failureCode ?? 'conflict' } };
        }
        if (input.action === 'disable') { row.enabled = false; row.status = 'restored'; row.revision++; }
        if (input.action === 'enable') { row.enabled = true; row.status = 'applied'; row.revision++; }
        if (input.action === 'sync') row.status = 'applied';
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
  await expect(page.getByRole('switch')).toHaveCount(6);
  await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toHaveCount(0);
  await expect(page.getByText(fixture.workspace.targets[0].config_root, { exact: true })).toHaveCount(0);
  for (const name of ['Claude Code', 'Gemini CLI', 'OpenCode', 'DSH Web', 'DSH Desktop']) await expect(page.getByRole('switch', { name: `${name} 托管`, exact: true })).toBeDisabled();
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
  await page.getByRole('button', { name: '重新接管并同步', exact: true }).click();
  await expect(page.getByRole('alertdialog')).toContainText('外部修改会先保存为恢复基线');
  expect(await page.evaluate(() => window.agentTest.actions)).toEqual([]);
  await page.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.agentTest.actions)).toEqual([]);
  await page.getByRole('button', { name: '重新接管并同步', exact: true }).click();
  await page.getByRole('button', { name: '确认变更', exact: true }).click();
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

for (const status of ['overridden', 'conflict']) {
  test(`explicit sync recovers ${status} through a confirmed action`, async ({ page }) => {
    await openSettings(page);
    await page.getByRole('button', { name: '开启托管', exact: true }).click();
    await page.evaluate(status => { window.agentTest.data.bindings[0].status = status; }, status);
    await page.getByRole('button', { name: '重新识别', exact: true }).click();
    await openSettings(page);
    await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toContainText('可重新接管');
    await page.getByRole('button', { name: '重新接管并同步', exact: true }).click();
    await expect(page.getByRole('alertdialog')).toContainText('后台同步不会自动抢回外部修改');
    expect(await page.evaluate(() => window.agentTest.actions)).toEqual([]);
    await page.getByRole('button', { name: '确认变更', exact: true }).click();
    await expect.poll(() => page.evaluate(() => window.agentTest.actions)).toEqual([fixture.sync]);
    expect(await page.evaluate(() => window.agentTest.enrolls.length)).toBe(1);
    await openSettings(page);
    await expect(page.getByText('配置已应用', { exact: true })).toBeVisible();
  });
}

test('Agent write conflict rereads state and does not replay the write', async ({ page }) => {
  const reads = await page.evaluate(() => { window.agentTest.failWrite = true; return window.agentTest.reads; });
  await openSettings(page);
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('托管记录版本已变化');
  await expect(page.getByRole('alert')).toContainText('已重新读取');
  await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toBeVisible();
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
      adapter_id: 'codex', revision: 1, model_scope: 'all-compatible', catalog_version: '0.159.3', status: 'applied',
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
  await expect(page.getByText('现有模型映射没有启用的候选供应商。', { exact: false })).toBeVisible();
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('');
  await expect(page.getByRole('button', { name: '开启托管', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.agentTest.enrolls)).toEqual([]);
});

test('DSH discovery details and all brand logos stay inside platform rows', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.data.targets.push({ id: 'dsh-fixture', agent: 'dsh', config_root: '/isolated/.dsh',
    source: 'home', supported: false, status: 'unsupported', catalog_version: '', templates: [] }); });
  await page.getByRole('button', { name: '重新识别' }).click();
  expect(await page.locator('.agent-brand img').evaluateAll(images => images.every(image => (image as HTMLImageElement).complete && (image as HTMLImageElement).naturalWidth > 0))).toBe(true);
  await page.getByRole('button', { name: 'DSH Web 托管设置', exact: true }).click();
  const region = page.getByRole('region', { name: 'DSH Web 托管设置' });
  await expect(region).toContainText('/isolated/.dsh');
  await expect(region).toContainText('尚无适配器');
  await expect(page.getByRole('switch', { name: 'DSH Web 托管', exact: true })).toBeDisabled();
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

for (const [agent, name] of [['claude', 'Claude Code'], ['dsh', 'DSH Web'], ['dsh-desktop', 'DSH Desktop']]) {
  test(`${name} enrolls all mapped aliases and restores through its platform switch`, async ({ page }) => {
    await page.evaluate(agent => {
      const id = `${agent}-fixture`;
      window.agentTest.data.targets.push({ id, agent, config_root: `/fixture/.${agent}`, source: 'home', supported: true, status: 'ready', catalog_version: '', templates: [] });
      window.agentTest.data.models.push({ id: 'mapped-without-metadata', tools: false, streaming: false, upstream_models: ['upstream'], available_targets: [id] },
        { id: 'disabled-provider', tools: true, streaming: true, upstream_models: ['unavailable'], available_targets: [] });
    }, agent);
    await page.getByRole('button', { name: '重新识别', exact: true }).click();
    const toggle = page.getByRole('switch', { name: `${name} 托管`, exact: true });
    await expect(toggle).toBeEnabled();
    await toggle.click();
    const select = page.getByLabel('默认模型', { exact: true });
    await expect(select.locator('option')).toHaveCount(4);
    await expect(select.locator('option[value="disabled-provider"]')).toBeDisabled();
    await select.selectOption('mapped-without-metadata');
    await page.getByRole('button', { name: '开启托管', exact: true }).click();
    await expect(toggle).toBeChecked();
    expect(await page.evaluate(() => window.agentTest.enrolls.at(-1))).toMatchObject({ target_id: `${agent}-fixture`, model: 'mapped-without-metadata', model_scope: 'all-mapped', approved: true });
    await toggle.click();
    await page.getByRole('button', { name: '确认变更', exact: true }).click();
    await expect(toggle).not.toBeChecked();
    expect(await page.evaluate(() => window.agentTest.actions.at(-1))).toMatchObject({ binding_id: `${agent}-fixture`, action: 'disable' });
  });
}

test('DSH Web and Desktop at the same home keep independent switches and actions', async ({ page }) => {
  await page.evaluate(() => {
    for (const agent of ['dsh', 'dsh-desktop']) {
      window.agentTest.data.targets.push({ id: agent, agent, config_root: '/fixture/.dsh', source: 'home', supported: true, status: 'ready', catalog_version: '', templates: [] });
      for (const model of window.agentTest.data.models) model.available_targets.push(agent);
    }
  });
  await page.getByRole('button', { name: '重新识别', exact: true }).click();
  const web = page.getByRole('switch', { name: 'DSH Web 托管', exact: true });
  const desktop = page.getByRole('switch', { name: 'DSH Desktop 托管', exact: true });
  await web.click();
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(web).toBeChecked();
  await expect(desktop).not.toBeChecked();
  await desktop.click();
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(web).toBeChecked();
  await expect(desktop).toBeChecked();
  expect(await page.evaluate(() => window.agentTest.enrolls.map(v => v.target_id))).toEqual(['dsh', 'dsh-desktop']);
  await web.click();
  await page.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(web).not.toBeChecked();
  await expect(desktop).toBeChecked();
  await page.getByRole('button', { name: 'DSH Desktop 托管设置', exact: true }).click();
  await page.getByRole('button', { name: '重新接管并同步', exact: true }).click();
  await page.getByRole('button', { name: '确认变更', exact: true }).click();
  expect(await page.evaluate(() => window.agentTest.actions)).toMatchObject([
    { binding_id: 'dsh', action: 'disable' }, { binding_id: 'dsh-desktop', action: 'sync' },
  ]);
});

for (const [code, message] of [
  ['agent_catalog_invalid', '无法生成托管模型目录'],
  ['agent_config_conflict', '托管配置或模型目录已变化'],
  ['agent_model_unavailable', '默认模型在当前托管范围内不可用'],
  ['agent_operation_failed', '不能据此判定为配置冲突'],
] as const) {
  test(`specific safe failure remains visible: ${code}`, async ({ page }) => {
    await page.evaluate(code => { window.agentTest.failWrite = true; window.agentTest.failureCode = code; }, code);
    await openSettings(page);
    await page.getByRole('button', { name: '开启托管', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText(message);
    await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toBeVisible();
    expect(await page.evaluate(() => window.agentTest.enrolls.length)).toBe(1);
  });
}

test('failed reread does not claim fresh state or replay enrollment', async ({ page }) => {
  await page.evaluate(() => { window.agentTest.failWrite = true; window.agentTest.failRead = true; });
  await openSettings(page);
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('重新读取也失败了');
  await expect(page.getByRole('alert')).not.toContainText('已重新读取');
  expect(await page.evaluate(() => window.agentTest.enrolls.length)).toBe(1);
});

test('partial enable exposes application failure even though policy is enabled', async ({ page }) => {
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  await page.getByRole('button', { name: '开启托管', exact: true }).click();
  await page.evaluate(() => { window.agentTest.data.bindings[0].enabled = false; window.agentTest.failWrite = true; window.agentTest.failureCode = 'agent_catalog_invalid'; });
  await page.getByRole('button', { name: '重新识别' }).click();
  await page.getByRole('switch', { name: 'Codex 托管', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('无法生成托管模型目录');
  await expect(page.getByRole('region', { name: 'Codex 托管设置' })).toBeVisible();
  await expect(page.getByLabel('Codex 托管状态', { exact: true })).toHaveText('应用失败');
  await expect(page.getByLabel('默认模型', { exact: true })).toHaveValue('public');
  await page.getByRole('button', { name: 'Codex 托管设置', exact: true }).click();
  await expect(page.getByLabel('Codex 托管状态', { exact: true })).toHaveText('应用失败');
  expect(await page.evaluate(() => window.agentTest.actions.length)).toBe(1);
});
