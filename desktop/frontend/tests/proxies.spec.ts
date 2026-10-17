import { expect, test } from '@playwright/test';
import fixture from '../../contract/proxies.json' with { type: 'json' };
import type { DesktopBridge, ProxyAction, ProxyWorkspace } from '../src/desktop/contract.generated';

declare global { interface Window { proxyTest: { data: ProxyWorkspace; actions: ProxyAction[]; reads: number; failure: string } } }

test.beforeEach(async ({ page }) => {
  await page.addInitScript(f => {
    const data: ProxyWorkspace = structuredClone(f.workspace);
    window.proxyTest = { data, actions: [], reads: 0, failure: '' };
    const result = <T,>(value: T) => ({ contract: f.contract, data: value, problem: null });
    const session = { instance_id: 'proxy-isolated', phase: 'ready', source: 'started', can_stop: true };
    window.go = { desktop: { Bridge: {
      Start: async () => result(session), Status: async () => result(session), Providers: async () => result([]),
      ProxyWorkspace: async () => { window.proxyTest.reads++; return result(data); },
      ManageProxy: async (input: ProxyAction) => {
        window.proxyTest.actions.push(structuredClone(input));
        if (window.proxyTest.failure) return { contract: f.contract, data: null, problem: { code: window.proxyTest.failure } };
        if (input.expected_revision !== data.revision) return { contract: f.contract, data: null, problem: { code: 'conflict' } };
        if (input.action === 'put') {
          const old = data.profiles.find(p => p.id === input.profile.id);
          data.profiles = data.profiles.filter(p => p.id !== input.profile.id);
          data.profiles.push({ ...input.profile, revision: (old?.revision ?? 0) + 1 });
        } else if (input.action === 'bind') {
          data.bindings = data.bindings.filter(b => b.provider_id !== input.binding.provider_id || b.key_id !== input.binding.key_id);
          data.bindings.push(input.binding);
        }
        data.revision++;
        return result(data);
      },
    } as unknown as DesktopBridge } };
    Object.assign(window, { runtime: { WindowMinimise: () => {}, WindowToggleMaximise: () => {}, WindowIsMaximised: async () => false, Quit: () => {} } });
  }, fixture);
  await page.goto('/');
  await page.getByRole('button', { name: '代理设置', exact: true }).click();
  await expect(page.getByRole('heading', { name: '代理设置', exact: true })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'demo 默认 代理' })).toHaveValue('proxy:edge');
});

test('node editor does not read secrets back and Key routing overrides the supplier', async ({ page }) => {
  await page.getByRole('button', { name: '添加代理', exact: true }).click();
  await page.getByLabel('ID', { exact: true }).fill('new-node');
  await page.getByLabel('名称', { exact: true }).fill('测试节点');
  const secret = 'type: http\nserver: 127.0.0.1\nport: 18888\npassword: synthetic-test-only';
  await page.getByRole('textbox', { name: '代理秘密配置' }).fill(secret);
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '代理秘密配置' })).toHaveCount(0);
  const row = page.getByRole('listitem').filter({ hasText: 'new-node' });
  await row.getByRole('button', { name: '编辑', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '代理秘密配置' })).toHaveValue('');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '代理秘密配置' })).toHaveCount(0);
  const key = page.getByRole('combobox', { name: 'demo second 代理' });
  await key.selectOption('inherit');
  await expect(key).toHaveValue('inherit');
  await key.selectOption('proxy:new-node');
  await expect(key).toHaveValue('proxy:new-node');
  await expect(page.getByRole('combobox', { name: 'demo 默认 代理' })).toHaveValue('proxy:edge');
  const actions = await page.evaluate(() => window.proxyTest.actions);
  expect(actions.map(a => a.expected_revision)).toEqual([4, 5, 6, 7]);
  expect(actions.every(a => a.approved)).toBe(true);
  expect(actions[0].secret).toBe(secret);
  expect(actions[1].secret).toBeNull();
  expect(actions[2].binding.mode).toBe('inherit');
  expect(actions[3].binding).toEqual({ provider_id: 'demo', key_id: 'second', mode: 'proxy', proxy_id: 'new-node' });
});

test('unknown writes lock mutations until an explicit reread without retrying', async ({ page }) => {
  await page.evaluate(() => { window.proxyTest.failure = 'unknown'; });
  const key = page.getByRole('combobox', { name: 'demo second 代理' });
  await key.selectOption('inherit');
  await expect(page.getByRole('alert')).toContainText('不要直接重复提交');
  await expect(key).toBeDisabled();
  await expect(page.getByRole('button', { name: '添加代理', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.proxyTest.actions.length)).toBe(1);
  await page.evaluate(() => { window.proxyTest.failure = ''; });
  await page.getByRole('button', { name: '重新读取', exact: true }).click();
  await expect(key).toBeEnabled();
  await expect(key).toHaveValue('direct');
  expect(await page.evaluate(() => window.proxyTest.actions.length)).toBe(1);
});
