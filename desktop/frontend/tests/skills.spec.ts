import { expect, test } from '@playwright/test';
import fixture from '../../contract/skills.json' with { type: 'json' };
import type { DesktopBridge, SkillAction, SkillQuery, SkillWorkspace } from '../src/desktop/contract.generated';

declare global { interface Window { skillTest: { data: SkillWorkspace; actions: SkillAction[]; queries: SkillQuery[]; reads: number; fail: boolean } } }
test.beforeEach(async ({ page }) => {
  await page.addInitScript(f => {
    const data = structuredClone(f.workspace) as SkillWorkspace;
    window.skillTest = { data, actions: [], queries: [], reads: 0, fail: false };
    const result = <T,>(value: T) => ({ contract: f.contract, data: value, problem: null });
    const session = { instance_id: 'skill-isolated', phase: 'ready', source: 'started', can_stop: true };
    window.go = { desktop: { Bridge: {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      SkillWorkspace: async () => { window.skillTest.reads++; return result(data); },
      SkillDetail: async (input: SkillQuery) => { window.skillTest.queries.push(input); return result({ ...f.detail, skill_id: input.skill_id || f.detail.skill_id, source_id: input.source_id, version: input.version || input.against_version || 1 }); },
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
  await expect(page.getByRole('button', { name: '我的 Skill · 1 个' })).toBeVisible();
});

test('saving creates a version only; existing mounts stay pinned', async ({ page }) => {
  await page.getByRole('button', { name: /example.*1 个已保存版本/ }).click();
  await expect(page.getByRole('combobox', { name: 'Skill 版本' })).toHaveValue('1');
  await expect(page.getByRole('button', { name: '从 Anchor 移除此 Skill' })).toBeDisabled();
  await page.getByRole('button', { name: '编辑文件', exact: true }).click();
  await page.getByRole('textbox', { name: '文件内容' }).fill('# Edited\n');
  await page.getByRole('button', { name: '保存为新版本', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions.length)).toBe(0);
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('原版本、其他位置的文件和快捷方式使用的版本都不变');
  await confirm.getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('status')).toContainText('操作已完成');
  const result = await page.evaluate(() => window.skillTest);
  expect(result.actions).toHaveLength(1);
  expect(result.actions[0]).toMatchObject({ action: 'save', version: 1, expected_revision: 3, approved: true });
  expect(result.data.mounts[0].version).toBe(1);
});

test('a Skill shows original files and Anchor shortcuts together, without storage jargon', async ({ page }, testInfo) => {
  await expect(page.getByRole('button', { name: '文件位置 · 2 处' })).toBeVisible();
  await expect(page.getByRole('button', { name: /中心库|本机来源|挂载/ })).toHaveCount(0);
  await page.getByRole('button', { name: /example.*1 个已保存版本.*2 处文件位置/ }).click();
  const locations = page.getByRole('region', { name: '此 Skill 的文件位置', exact: true });
  await expect(locations.getByText('原位置文件', { exact: true })).toBeVisible();
  await expect(locations.getByText('Anchor 快捷方式', { exact: true })).toBeVisible();
  await expect(locations.getByText(fixture.workspace.sources[0].path, { exact: true })).toBeVisible();
  await expect(locations.getByText(fixture.workspace.mounts[0].path, { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 900, height: 1000 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('skill-positions.png') });
  await page.setViewportSize({ width: 560, height: 920 });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('skill-positions-narrow-dark.png') });
});

test('40 saved Skills and 47 original paths plus 7 shortcuts are presented as 40 Skills and 54 positions', async ({ page }) => {
  await page.evaluate(() => {
    const d = window.skillTest.data, skill = d.library[0], source = d.sources[0], mount = d.mounts[0];
    d.library = Array.from({ length: 40 }, (_, i) => ({ ...skill, id: `skill-${i}`, name: `example-${i}` }));
    d.sources = Array.from({ length: 47 }, (_, i) => ({ ...source, id: `source-${i}`, skill_id: `skill-${i % 36}`, path: `/fixture/original/${i}` }));
    d.mounts = Array.from({ length: 7 }, (_, i) => ({ ...mount, id: `mount-${i}`, skill_id: `skill-${36 + i % 4}`, path: `/fixture/shortcut/${i}` }));
  });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await expect(page.getByRole('button', { name: '我的 Skill · 40 个' })).toBeVisible();
  await page.getByRole('button', { name: '文件位置 · 54 处' }).click();
  await expect(page.getByText('原位置文件 47 处 · Anchor 快捷方式 7 处', { exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: '文件位置列表' }).getByRole('listitem')).toHaveCount(54);
  await page.getByRole('combobox', { name: '筛选 Skill' }).selectOption('skill-0');
  await expect(page.getByRole('list', { name: '文件位置列表' }).getByRole('listitem')).toHaveCount(2);
  await expect(page.getByText('原位置文件 2 处 · Anchor 快捷方式 0 处', { exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: '筛选 Skill' }).selectOption('skill-39');
  await expect(page.getByRole('list', { name: '文件位置列表' }).getByRole('listitem')).toHaveCount(1);
  await expect(page.getByText('原位置文件 0 处 · Anchor 快捷方式 1 处', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});

test('adding to an Agent explains the selected version and never claims it was loaded', async ({ page }) => {
  await page.getByRole('button', { name: /example.*1 个已保存版本/ }).click();
  await page.getByRole('button', { name: '添加到 Agent / 项目', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('combobox', { name: '目标 Agent / 项目' })).toHaveValue('scope-fixture');
  await dialog.getByRole('button', { name: '确认添加位置…' }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('以后保存新版本不会自动更新这里');
  await expect(confirm).toContainText('添加成功不代表 Agent 已加载');
  await expect(confirm).toContainText(fixture.workspace.scopes[0].root);
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await dialog.getByRole('button', { name: '确认添加位置…' }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toEqual([expect.objectContaining({ action: 'link', skill_id: 'skill-fixture', version: 1, scope_id: 'scope-fixture', expected_revision: 3, approved: true })]);
});

test('inspecting a position from a Skill opens its filtered differences without writing', async ({ page }) => {
  await page.evaluate(() => {
    const skill = window.skillTest.data.library[0];
    skill.versions.push({ ...skill.versions[0], number: 2, parent: 1 });
  });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await page.getByRole('button', { name: /example.*2 个已保存版本/ }).click();
  await page.getByRole('button', { name: '查看改动', exact: true }).click();
  await expect(page.getByRole('button', { name: '文件位置 · 2 处' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('combobox', { name: '筛选 Skill' })).toHaveValue('skill-fixture');
  await expect(page.getByRole('region', { name: '此处文件的改动', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '对比 Anchor 最新版' }).click();
  await expect(page.getByRole('region', { name: '此处文件的改动', exact: true })).toContainText('对比 Anchor 已保存的 v2');
  expect(await page.evaluate(() => window.skillTest.queries.at(-1))).toMatchObject({ source_id: 'source-fixture', against_version: 2 });
  await page.getByRole('combobox', { name: '筛选 Skill' }).selectOption('');
  await expect(page.getByRole('region', { name: '此处文件的改动', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});

test('unreadable files that have not been saved remain visible instead of inflating the Skill count', async ({ page }) => {
  await page.evaluate(() => {
    const d = window.skillTest.data;
    d.library = []; d.mounts = [];
    d.sources[0] = { ...d.sources[0], skill_id: '', baseline: 0, hash: '', status: 'unsafe' };
  });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await expect(page.getByRole('button', { name: '我的 Skill · 0 个' })).toBeVisible();
  await page.getByRole('button', { name: '文件位置 · 1 处' }).click();
  await expect(page.getByText('文件无法读取、链接损坏或文件类型不支持', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '保存到 Anchor', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: '改用 Anchor 副本', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});

test('source conflict does not retry a write or propose replacing originals', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.fail = true; });
  await page.getByRole('button', { name: '文件位置 · 2 处' }).click();
  await expect(page.getByText('此处文件有新改动', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '保存此处改动', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('只复制保存到 Anchor，不修改此处文件或任何快捷方式');
  await confirm.getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('alert')).toContainText('未覆盖外部变化');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(1);
  expect(await page.evaluate(() => window.skillTest.actions[0].expected_hash)).toBe(fixture.workspace.sources[0].hash);
  expect(await page.evaluate(() => window.skillTest.reads)).toBeGreaterThan(1);
});

test('missing mount is not automatically recreated and unlink requires confirmation', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.data.mounts[0].status = 'missing'; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await page.getByRole('button', { name: '文件位置 · 2 处' }).click();
  await expect(page.getByText('此处文件已不存在', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '移除快捷方式', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText('不恢复独立文件');
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '移除快捷方式', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('原位置文件 1 处 · Anchor 快捷方式 0 处', { exact: true })).toBeVisible();
});

test('cleanup shows the exact source and archive-before-replacement promise', async ({ page }) => {
  await page.getByRole('button', { name: '文件位置 · 2 处' }).click();
  await page.getByRole('button', { name: '改用 Anchor 副本', exact: true }).click();
  const confirm = page.getByRole('alertdialog');
  await expect(confirm).toContainText(fixture.workspace.sources[0].path);
  await expect(confirm).toContainText('完整保存到 Anchor 并核对');
  await expect(confirm).toContainText('再删除此处已保存的原文件');
  await expect(confirm).toContainText('以后在 Anchor 编辑不会自动更新此处版本');
  await expect(confirm).toContainText('不操作也能继续使用原文件');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await confirm.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
  await page.getByRole('button', { name: '改用 Anchor 副本', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('status')).toContainText('操作已完成');
  expect(await page.evaluate(() => window.skillTest.actions)).toEqual([fixture.cleanup]);
});

test('archived sources need no import and remain optional to clean', async ({ page }) => {
  await page.evaluate(() => { window.skillTest.data.sources[0].status = 'in_sync'; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await page.getByRole('button', { name: '文件位置 · 2 处' }).click();
  await expect(page.getByText('与 Anchor 保存的内容相同', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '保存此处改动', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '改用 Anchor 副本', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});

test('project discovery is automatic and manual paths are only a supplement', async ({ page }) => {
  await page.getByText('使用说明', { exact: true }).click();
  await expect(page.getByText(/常见项目目录会自动识别/)).toBeVisible();
  await page.evaluate(() => { window.skillTest.data.warnings = ['project_search_limit']; });
  await page.getByRole('button', { name: '重新扫描' }).click();
  await expect(page.getByRole('status')).toContainText('部分项目尚未检查');
  await page.getByRole('button', { name: '补充项目', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText('非常用位置');
  expect(await page.evaluate(() => window.skillTest.actions)).toHaveLength(0);
});
