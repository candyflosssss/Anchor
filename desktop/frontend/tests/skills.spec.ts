import { expect, test } from '@playwright/test';
import fixture from '../../contract/skills.json' with { type: 'json' };
import type { DesktopBridge, SkillAction, SkillQuery, SkillWorkspace } from '../src/desktop/contract.generated';

declare global { interface Window { skillTest: { data: SkillWorkspace; actions: SkillAction[]; reads: number; fail: boolean } } }
test.beforeEach(async ({ page }) => {
  await page.addInitScript(f => {
    const data = structuredClone(f.workspace) as SkillWorkspace;
    window.skillTest = { data, actions: [], reads: 0, fail: false };
    const result = <T,>(value: T) => ({ contract: f.contract, data: value, problem: null });
    const session = { instance_id: 'skill-isolated', phase: 'ready', source: 'started', can_stop: true };
    window.go = { desktop: { Bridge: {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      SkillWorkspace: async () => { window.skillTest.reads++; return result(data); },
      SkillDetail: async (input: SkillQuery) => result({ ...f.detail, skill_id: input.skill_id || f.detail.skill_id, source_id: input.source_id, version: input.version || 1 }),
      ManageSkill: async (input: SkillAction) => {
        window.skillTest.actions.push(input);
        if (window.skillTest.fail) return { contract: f.contract, data: null, problem: { code: 'skill_changed' } };
        if (input.action === 'save') data.library[0].versions.push({ ...data.library[0].versions[0], number: 2, parent: 1, baseline: 1, origin: 'save' });
        if (input.action === 'unlink') data.mounts = [];
        data.revision++;
        return result(data);
      },
    } as unknown as DesktopBridge } };
    Object.assign(window, { runtime: { WindowMinimise: () => {}, WindowToggleMaximise: () => {}, WindowIsMaximised: async () => false, Quit: () => {} } });
  }, fixture);
  await page.goto('/');
  await page.getByRole('button', { name: 'Skills', exact: true }).click();
  await expect(page.getByRole('button', { name: '中心库 · 1' })).toBeVisible();
});

test('saving creates a version only; existing mounts stay pinned', async ({ page }) => {
  await page.getByRole('button', { name: /example.*1 个版本/ }).click();
  await expect(page.getByRole('combobox', { name: 'Skill 版本' })).toHaveValue('1');
  await expect(page.getByRole('button', { name: '删除库中 Skill' })).toBeDisabled();
  await page.getByRole('button', { name: '编辑文件', exact: true }).click();
  await page.getByRole('textbox', { name: '文件内容' }).fill('# Edited\n');
  await page.getByRole('button', { name: '保存为新版本', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions.length)).toBe(0);
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('原版本、项目原件和现有挂载保持不变');
  await confirm.getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('status')).toContainText('操作已完成');
  const result = await page.evaluate(() => window.skillTest);
  expect(result.actions).toHaveLength(1);
  expect(result.actions[0]).toMatchObject({ action: 'save', version: 1, expected_revision: 3, approved: true });
  expect(result.data.mounts[0].version).toBe(1);
});

test('source conflict does not retry a write or propose replacing originals', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.fail = true; });
  await page.getByRole('button', { name: '本机来源 · 1' }).click();
  await expect(page.getByText('来源已变化', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '导入修订', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('只复制到中心库，不修改原件或现有链接');
  await confirm.getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('alert')).toContainText('未覆盖外部变化');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(1);
  expect(await page.evaluate(() => window.skillTest.actions[0].expected_hash)).toBe(fixture.workspace.sources[0].hash);
  expect(await page.evaluate(() => window.skillTest.reads)).toBeGreaterThan(1);
});

test('missing mount is not automatically recreated and unlink requires confirmation', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.data.mounts[0].status = 'missing'; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await page.getByRole('button', { name: '挂载 · 1' }).click();
  await expect(page.getByText('原位置已缺失', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '解除挂载', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '解除挂载', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('暂无 Anchor 管理的链接')).toBeVisible();
});

test('cleanup shows the exact source and archive-before-replacement promise', async ({ page }) => {
  await page.getByRole('button', { name: '本机来源 · 1' }).click();
  await page.getByRole('button', { name: '清理原件', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText(fixture.workspace.sources[0].path);
  await expect(confirm).toContainText('完整复制到 Anchor 库并核对');
  await expect(confirm).toContainText('不清理也能继续使用');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '清理原件', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('status')).toContainText('操作已完成');
  expect(await page.evaluate(() => window.skillTest.actions)).toEqual([fixture.cleanup]);
});

test('archived sources need no import and remain optional to clean', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.data.sources[0].status = 'in_sync'; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await page.getByRole('button', { name: '本机来源 · 1' }).click();
  await expect(page.getByText('已收录 · 内容一致', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '导入修订', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '清理原件', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});

test('project discovery is automatic and manual paths are only a supplement', async ({ page }) => {
  await expect(page.getByText(/常见项目目录会自动识别/)).toBeVisible();
  await page.evaluate(() => { window.skillTest.data.warnings = ['project_search_limit']; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await expect(page.getByRole('status')).toContainText('部分项目尚未检查');
  await page.getByRole('button', { name: '补充项目', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('非常用位置');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});
