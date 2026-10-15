import { test, expect } from '@playwright/test';
import fixture from '../../contract/connections.json' with { type: 'json' };
import type { DesktopBridge, SSHConnectionInput, SessionInfo } from '../src/desktop/contract.generated';

test('UI enrolls, switches and preserves current device after failure', async ({ page }) => {
  await page.addInitScript(({ fixture }) => {
    const contract = fixture.contract;
    let current: SessionInfo = { instance_id: 'local-fixture', profile_name: '@local', transport: 'local', target: '本机', source: 'reused', phase: 'ready', can_stop: true };
    let added = false;
    const calls: SSHConnectionInput[] = [];
    Object.assign(window, { connectionTest: { calls } });
    const operations = {
      Start: async () => ({ contract, data: current, problem: null }),
      Status: async () => ({ contract, data: current, problem: null }),
      Providers: async () => ({ contract, data: [], problem: null }),
      Connections: async () => ({ contract, data: fixture.profiles.data.filter(row => row.transport === 'local' || added).map(row => ({ ...row, active: row.name === current.profile_name })), problem: null }),
      ConnectSSH: async (input: SSHConnectionInput) => {
        calls.push(input);
        await new Promise(resolve => setTimeout(resolve, 250));
        if (input.target === 'unreachable') return { contract, data: null, problem: { code: 'ssh_unreachable' } };
        added = true; current = fixture.session.data;
        return { contract, data: current, problem: null };
      },
      UseConnection: async ({ name }: { name: string }) => {
        current = name === '@local' ? { ...current, profile_name: '@local', instance_id: 'local-fixture', target: '本机', transport: 'local', source: 'reused' } : fixture.session.data;
        return { contract, data: current, problem: null };
      },
    };
    const api = new Proxy(operations, { get: (target, key) => target[key as keyof typeof operations] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge;
    window.go = { desktop: { Bridge: api } };
  }, { fixture });
  await page.goto('/');
  await page.getByRole('button', { name: '设备连接', exact: true }).click();
  await page.getByLabel('连接名称').fill('dev-server');
  await page.getByLabel('SSH 目标').fill('developer@192.0.2.10');
  await page.getByLabel('使用密码登录').check();
  await page.getByLabel('SSH 密码', { exact: true }).fill('browser-fixture-password');
  await page.getByLabel('信任首次连接的主机密钥').check();
  await page.getByRole('button', { name: '添加并连接', exact: true }).click();
  await expect(page.getByRole('button', { name: '正在连接…' })).toBeDisabled();
  await expect(page.getByLabel('SSH 密码', { exact: true })).toHaveValue('');
  await expect(page.locator('.connection-profile').filter({ hasText: 'dev-server' })).toContainText('当前设备');
  await expect(page.getByLabel('连接名称')).toHaveValue('');
  expect(await page.evaluate(() => (window as unknown as { connectionTest: { calls: SSHConnectionInput[] } }).connectionTest.calls.length)).toBe(1);
  await page.getByLabel('连接名称').fill('failed-server');
  await page.getByLabel('SSH 目标').fill('unreachable');
  await page.getByLabel('SSH 密码', { exact: true }).fill('retry-fixture-password');
  await page.getByRole('button', { name: '添加并连接', exact: true }).click();
  await expect(page.getByText('无法访问 SSH 目标，请检查地址、端口和网络。')).toBeVisible();
  await expect(page.locator('.connection-profile').filter({ hasText: 'dev-server' })).toContainText('当前设备');
  await page.locator('.connection-profile').filter({ hasText: '本机 Anchor 核心' }).getByRole('button', { name: '连接', exact: true }).click();
  await expect(page.locator('.connection-profile').filter({ hasText: '本机 Anchor 核心' })).toContainText('当前设备');
});

test('connection screen is usable when local core is offline', async ({ page }) => {
  await page.addInitScript(() => {
    const contract = 'anchor-desktop-v1';
    window.go = { desktop: { Bridge: new Proxy({}, { get: (_, key) => async () => key === 'Connections' ? ({ contract, data: [], problem: null }) : ({ contract, data: null, problem: { code: 'offline' } }) }) as DesktopBridge } };
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible();
  await page.getByRole('button', { name: '设备连接', exact: true }).click();
  await expect(page.getByLabel('SSH 目标')).toBeEnabled();
  await expect(page.getByRole('button', { name: '添加并连接' })).toBeEnabled();
});

async function quickSwitchHost(page: import('@playwright/test').Page, authentication = 'key') {
  await page.addInitScript(({ fixture, authentication }) => {
    const contract = fixture.contract;
    let current: SessionInfo = { ...fixture.session.data, profile_name: '@local', instance_id: 'local-fixture', transport: 'local', target: '本机', source: 'reused' };
    const state = { calls: [] as { name: string; password: string }[], fail: false, listFail: false };
    Object.assign(window, { quickSwitchTest: state });
    const api = {
      Start: async () => ({ contract, data: current, problem: null }),
      Status: async () => ({ contract, data: current, problem: null }),
      Providers: async () => ({ contract, data: [], problem: null }),
      Connections: async () => state.listFail ? ({ contract, data: null, problem: { code: 'read_error' } }) : ({ contract, data: fixture.profiles.data.map(row => ({ ...row, authentication: row.transport === 'ssh' ? authentication : 'owner', active: row.name === current.profile_name })), problem: null }),
      UseConnection: async (input: { name: string; password: string }) => {
        state.calls.push(input);
        await new Promise(resolve => setTimeout(resolve, 350));
        if (state.fail || (authentication === 'password' && input.name !== '@local' && input.password !== 'quick-switch-test-password')) return { contract, data: null, problem: { code: 'ssh_auth' } };
        current = input.name === '@local' ? { ...current, profile_name: '@local', instance_id: 'local-fixture', transport: 'local', target: '本机', source: 'reused' } : fixture.session.data;
        return { contract, data: current, problem: null };
      },
    };
    window.go = { desktop: { Bridge: new Proxy(api, { get: (target, key) => target[key as keyof typeof api] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
  }, { fixture, authentication });
  await page.goto('/');
}

test('header switches devices in place, hides instance IDs and refreshes the connection screen', async ({ page }) => {
  await quickSwitchHost(page);
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  const panel = page.getByRole('dialog', { name: '切换设备' });
  await expect(panel).not.toContainText('local-fixture');
  await expect(panel).not.toContainText('remote-fixture');
  await expect(panel.getByRole('button', { name: '切换到本机', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await panel.getByRole('button', { name: '切换到dev-server' }).click();
  await expect(panel.getByRole('button', { name: '切换到本机', exact: true })).toBeDisabled();
  await expect(panel.getByRole('button', { name: '停止核心', exact: true })).toBeDisabled();
  await expect(panel).toBeHidden();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '供应商', exact: true })).toBeVisible();
  await page.getByRole('button', { name: /dev-server · 已连接/ }).click();
  await expect(panel.getByRole('button', { name: '切换到dev-server' })).toHaveAttribute('aria-pressed', 'true');
  await panel.getByRole('button', { name: '管理设备连接' }).click();
  await expect(page.getByRole('heading', { name: '设备连接' })).toBeVisible();
  await page.getByRole('button', { name: /dev-server · 已连接/ }).click();
  await panel.getByRole('button', { name: '切换到本机', exact: true }).click();
  await expect(page.locator('.connection-profile').filter({ hasText: '本机 Anchor 核心' })).toContainText('当前设备');
  await expect(page.locator('.connection-profile').filter({ hasText: 'dev-server' })).not.toContainText('当前设备');
});

test('header password retry preserves the current device and clears transient credentials', async ({ page }) => {
  await quickSwitchHost(page, 'password');
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  const panel = page.getByRole('dialog', { name: '切换设备' });
  await panel.getByRole('button', { name: '切换到dev-server' }).click();
  await panel.getByLabel('dev-server 的 SSH 密码').fill('wrong-test-password');
  await panel.getByRole('button', { name: '连接设备', exact: true }).click();
  await expect(panel.getByLabel('dev-server 的 SSH 密码')).toHaveValue('');
  await expect(panel.getByRole('alert')).toContainText('SSH 鉴权失败');
  await expect(page.getByRole('button', { name: /本机实例 · 已连接/ })).toBeVisible();
  await panel.getByLabel('dev-server 的 SSH 密码').fill('discard-test-password');
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await panel.getByRole('button', { name: '切换到dev-server' }).click();
  await expect(panel.getByLabel('dev-server 的 SSH 密码')).toHaveValue('');
  await panel.getByLabel('dev-server 的 SSH 密码').fill('quick-switch-test-password');
  await panel.getByRole('button', { name: '连接设备', exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('test-password');
});

test('dismissing a pending header switch still publishes the new session', async ({ page }) => {
  await quickSwitchHost(page);
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await page.getByRole('button', { name: '切换到dev-server' }).click();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: '切换设备' })).toBeHidden();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
});

test('header can retry a failed device list read', async ({ page }) => {
  await quickSwitchHost(page);
  await page.evaluate(() => { (window as unknown as { quickSwitchTest: { listFail: boolean } }).quickSwitchTest.listFail = true; });
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  const panel = page.getByRole('dialog', { name: '切换设备' });
  await expect(panel.getByRole('alert')).toBeVisible();
  await page.evaluate(() => { (window as unknown as { quickSwitchTest: { listFail: boolean } }).quickSwitchTest.listFail = false; });
  await panel.getByRole('button', { name: '刷新设备列表' }).click();
  await expect(panel.getByRole('button', { name: '切换到dev-server' })).toBeVisible();
});
