import { expect, test, type Page } from '@playwright/test';
import catalogFixture from '../../contract/provider-models.json' with { type: 'json' };
import workspaceFixture from '../../contract/provider-workspace.json' with { type: 'json' };
import mappingFixture from '../../contract/provider-model-mapping.json' with { type: 'json' };
import { CONTRACT, type DesktopBridge, type Provider, type Workspace } from '../src/desktop/contract.generated';

declare global {
  interface Window {
    providerSetupTest: { saves: number; discoveries: number; imports: number; acceptedKey: boolean; events: string[]; catalogFailure: string; readError: boolean; writeError: 'unknown' | 'conflict' | null };
  }
}
async function host(page: Page) {
  await page.addInitScript(({ contract, catalog, workspace, mapping }) => {
    const counters: Window['providerSetupTest'] = window.providerSetupTest = { saves: 0, discoveries: 0, imports: 0, acceptedKey: false, events: [] as string[], catalogFailure: '', readError: false, writeError: null as 'unknown' | 'conflict' | null };
    const providers: Provider[] = [];
    const details: Record<string, Workspace> = {};
    const session = { instance_id: 'setup-test-instance', phase: 'ready', source: 'started', profile_name: '@local', transport: 'local', target: '本机', can_stop: true };
    const api: Partial<DesktopBridge> = {
      Start: async () => ({ contract, data: session, problem: null }),
      Status: async () => ({ contract, data: session, problem: null }),
      Providers: async () => ({ contract, data: structuredClone(providers), problem: null }),
      ProviderWorkspace: async input => ({ contract, data: structuredClone(details[input.provider_id]), problem: null }),
      SaveProvider: async input => {
        counters.saves++; counters.events.push(`save:${input.expected_revision}`);
        counters.acceptedKey = input.secret === 'synthetic-onboarding-key';
        await new Promise(resolve => setTimeout(resolve, 150));
        if (counters.writeError === 'conflict') return { contract, data: null, problem: { code: 'conflict' } };
        const old = providers.find(p => p.id === input.provider.id);
        if ((old?.revision ?? 0) !== input.expected_revision) return { contract, data: null, problem: { code: 'conflict' } };
        const saved = { ...input.provider, revision: input.expected_revision + 1, has_secret: input.secret === null ? old?.has_secret ?? false : !!input.secret };
        if (old) Object.assign(old, saved); else providers.push(saved);
        details[saved.id] = { ...structuredClone(workspace), ...(details[saved.id] ?? {}), provider_id: saved.id, revision: saved.revision, keys: saved.has_secret ? structuredClone(workspace.keys) : [] };
        return counters.writeError === 'unknown' ? { contract, data: null, problem: { code: 'unknown' } } : { contract, data: structuredClone(saved), problem: null };
      },
      DiscoverProviderModels: async input => {
        counters.discoveries++; counters.events.push(`discover:${input.expected_revision}`);
        await new Promise(resolve => setTimeout(resolve, 150));
        if (counters.readError) return { contract, data: null, problem: { code: 'unknown' } };
        if (providers.find(p => p.id === input.provider_id)?.revision !== input.expected_revision) return { contract, data: null, problem: { code: 'conflict' } };
        const result = { ...structuredClone(catalog), provider_id: input.provider_id, revision: input.expected_revision, key_id: input.key_id };
        if (counters.catalogFailure) Object.assign(result, { http_status: 401, models: [], entries: [], failure: counters.catalogFailure, keys: result.keys.map(k => ({ ...k, http_status: 401, failure: counters.catalogFailure })) });
        return { contract, data: result, problem: null };
      },
      ProviderAction: async input => {
        counters.imports++;
        const detail = details[input.provider_id];
        if (input.expected_revision !== detail.revision || input.action !== 'model-put') return { contract, data: null, problem: { code: 'conflict' } };
        detail.models.push({ ...mapping, id: input.target, upstream: input.model, key_ids: input.key_ids ?? [], candidate_count: 1, candidate_index: 0 });
        return { contract, data: structuredClone(detail), problem: null };
      },
    };
    window.go = { desktop: { Bridge: new Proxy(api, { get: (target, name) => Reflect.get(target, name) ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as DesktopBridge } };
  }, { contract: CONTRACT, catalog: catalogFixture.success, workspace: workspaceFixture, mapping: mappingFixture });
  await page.goto('/');
  await page.getByRole('button', { name: '添加供应商', exact: true }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('供应商名称 / ID').fill('一站式供应商');
  await dialog.getByLabel('API 根地址', { exact: true }).fill('https://api.example.com/v1');
  return dialog;
}

test('creates with a Key, fetches after saving and imports in the same dialog', async ({ page }) => {
  const dialog = await host(page);
  await dialog.getByLabel('API Key（可选）').fill('synthetic-onboarding-key');
  await expect(dialog.getByLabel('API Key（可选）')).toHaveAttribute('type', 'password');
  await dialog.getByRole('button', { name: '添加并获取模型', exact: true }).dblclick();
  await expect(dialog.getByLabel('上游模型 ID').locator(':scope > li')).toHaveCount(2);
  expect(await page.evaluate(() => window.providerSetupTest.events)).toEqual(['save:0', 'discover:1']);
  expect(await page.evaluate(() => window.providerSetupTest.acceptedKey)).toBe(true);
  await expect(dialog.getByLabel('API Key（可选）')).toHaveCount(0);
  await dialog.getByRole('button', { name: /^全部导入/ }).dblclick();
  await expect(dialog.getByRole('status').filter({ hasText: '已导入 2 / 2' })).toBeVisible();
  expect(await page.evaluate(() => window.providerSetupTest.imports)).toBe(2);
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  await expect(dialog).toBeHidden();
  const storage = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }));
  expect(storage).not.toContain('synthetic-onboarding-key');
  expect(await page.content()).not.toContain('synthetic-onboarding-key');
});

test('optional Key and cancellation do not trigger model discovery', async ({ page }) => {
  const dialog = await host(page);
  await dialog.getByLabel('API Key（可选）').fill('discard-on-cancel');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.providerSetupTest.saves)).toBe(0);
  await page.getByRole('button', { name: '添加供应商', exact: true }).first().click();
  await expect(dialog.getByLabel('API Key（可选）')).toHaveValue('');
  await dialog.getByLabel('供应商名称 / ID').fill('无Key供应商');
  await dialog.getByLabel('API 根地址', { exact: true }).fill('https://api.example.com/v1');
  await dialog.getByRole('button', { name: '添加供应商', exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(await page.evaluate(() => ({ saves: window.providerSetupTest.saves, discoveries: window.providerSetupTest.discoveries }))).toEqual({ saves: 1, discoveries: 0 });
});

test('catalog authentication failure preserves the provider and allows Key correction', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const dialog = await host(page);
  await page.evaluate(() => { window.providerSetupTest.catalogFailure = 'authentication'; document.documentElement.classList.add('dark'); });
  await dialog.getByLabel('API Key（可选）').fill('synthetic-onboarding-key');
  await dialog.getByRole('button', { name: '添加并获取模型', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('上游拒绝目录授权');
  await dialog.getByRole('button', { name: '修改配置 / Key', exact: true }).click();
  await dialog.getByRole('button', { name: '替换密钥', exact: true }).click();
  await expect(dialog.getByLabel('新的 API 密钥')).toHaveValue('');
  await dialog.getByLabel('新的 API 密钥').fill('synthetic-onboarding-key');
  await page.evaluate(() => { window.providerSetupTest.catalogFailure = ''; });
  await dialog.getByRole('button', { name: '保存并获取模型', exact: true }).click();
  await expect(dialog.getByLabel('上游模型 ID').locator(':scope > li')).toHaveCount(2);
  expect(await page.evaluate(() => window.providerSetupTest.events)).toEqual(['save:0', 'discover:1', 'save:1', 'discover:2']);
  expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  const bounds = await dialog.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.width).toBeLessThanOrEqual(390);
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  await expect(dialog).toBeHidden();
});

test('a failed catalog read can be retried without replaying creation', async ({ page }) => {
  const dialog = await host(page);
  await page.evaluate(() => { window.providerSetupTest.readError = true; });
  await dialog.getByLabel('API Key（可选）').fill('synthetic-onboarding-key');
  await dialog.getByRole('button', { name: '添加并获取模型', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('未能读取模型目录');
  await expect(dialog).not.toContainText('写入结果未知');
  await page.evaluate(() => { window.providerSetupTest.readError = false; });
  await dialog.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await expect(dialog.getByLabel('上游模型 ID').locator(':scope > li')).toHaveCount(2);
  expect(await page.evaluate(() => window.providerSetupTest.events)).toEqual(['save:0', 'discover:1', 'discover:1']);
});

for (const writeError of ['unknown', 'conflict'] as const) test(`${writeError} creation does not fetch models or replay writes`, async ({ page }) => {
  const dialog = await host(page);
  await page.evaluate(value => { window.providerSetupTest.writeError = value; }, writeError);
  await dialog.getByLabel('API Key（可选）').fill('synthetic-onboarding-key');
  await dialog.getByRole('button', { name: '添加并获取模型', exact: true }).click();
  await expect(page.getByRole('alert').first()).toContainText(writeError === 'unknown' ? '写入结果未知' : '配置已被其他操作修改');
  expect(await page.evaluate(() => ({ saves: window.providerSetupTest.saves, discoveries: window.providerSetupTest.discoveries }))).toEqual({ saves: 1, discoveries: 0 });
  expect(await page.content()).not.toContain('synthetic-onboarding-key');
});
