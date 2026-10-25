import { test, expect } from '@playwright/test';
import fixture from '../../contract/connections.json' with { type: 'json' };
import type { DesktopBridge, SSHConnectionInput, SessionInfo, SelectConnection } from '../src/desktop/contract.generated';

test('UI enrolls, switches and preserves current device after failure', async ({ page }) => {
  await page.addInitScript(({ fixture }) => {
    const contract = fixture.contract;
    let current: SessionInfo = { instance_id: 'local-fixture', profile_name: '@local', transport: 'local', target: '本机', version: '0.1.0-dev.desktop.58', source: 'reused', phase: 'ready', can_stop: true };
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
        current = name === '@local' ? { ...current, profile_name: '@local', instance_id: 'local-fixture', target: '本机', transport: 'local', version: '0.1.0-dev.desktop.58', source: 'reused' } : fixture.session.data;
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

for (const stage of ['startup', 'disconnected'] as const) {
  test(`offline reconnect opens password recovery after SSH auth failure at ${stage}`, async ({ page }) => {
    await page.addInitScript(({ fixture, stage }) => {
      const contract = fixture.contract;
      const state = { starts: 0, authFailed: stage === 'startup', calls: [] as { name: string; password: string }[] };
      Object.assign(window, { reconnectTest: state });
      const failure = () => ({ contract, data: null, problem: { code: 'ssh_auth' } });
      const operations = {
        Start: async () => { state.starts++; return state.authFailed ? failure() : fixture.session; },
        Status: async () => state.authFailed ? failure() : fixture.session,
        Providers: async () => ({ contract, data: [], problem: null }),
        Connections: async () => ({ contract, data: fixture.profiles.data.map(row => ({ ...row, authentication: row.transport === 'ssh' ? 'password' : 'owner' })), problem: null }),
        UseConnection: async (input: { name: string; password: string }) => {
          state.calls.push(input);
          await new Promise(resolve => setTimeout(resolve, 250));
          if (input.name !== 'dev-server' || input.password !== 'reconnect-fixture-password') return failure();
          state.authFailed = false;
          return fixture.session;
        },
      };
      window.go = { desktop: { Bridge: new Proxy(operations, { get: (target, key) => target[key as keyof typeof operations] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
    }, { fixture, stage });
    await page.goto('/');
    if (stage === 'disconnected') {
      await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
      await page.evaluate(() => { (window as unknown as { reconnectTest: { authFailed: boolean } }).reconnectTest.authFailed = true; });
    }
    await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible({ timeout: 8000 });
    await expect(page.getByText('连接目标：dev-server（SSH：developer@192.0.2.10）', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: /dev-server · 未连接/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /本机实例 · 未连接/ })).toHaveCount(0);
    await expect(page.getByRole('button', { name: '恢复本机核心', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: '重新连接当前设备', exact: true }).click();
    await expect(page.getByRole('heading', { name: '设备连接', exact: true })).toBeVisible();
    await expect(page.getByText('已选设备（未连接）', { exact: true })).toBeVisible();
    expect(await page.evaluate(() => (window as unknown as { reconnectTest: { starts: number; calls: unknown[] } }).reconnectTest)).toMatchObject({ starts: 1, calls: [] });
    const device = page.locator('.connection-profile').filter({ hasText: 'dev-server' });
    const password = device.getByLabel('dev-server 的 SSH 密码');
    await password.fill('wrong-fixture-password');
    await device.getByRole('button', { name: /^(重新连接|连接)$/ }).click();
    await expect(password).toHaveValue('');
    await expect(page.getByRole('alert')).toContainText('SSH 鉴权失败');
    await password.fill('reconnect-fixture-password');
    await device.getByRole('button', { name: /^(重新连接|连接)$/ }).click();
    await expect(device.getByRole('button', { name: /^(重新连接|连接)$/ })).toBeDisabled();
    await expect(password).toHaveValue('');
    await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
    await page.getByRole('button', { name: '供应商', exact: true }).click();
    await expect(page.getByRole('heading', { name: '供应商', exact: true })).toBeVisible();
    expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('fixture-password');
  });
}

test('offline local reconnect retries startup and also offers device management', async ({ page }) => {
  await page.addInitScript(() => {
    let starts = 0;
    const contract = 'anchor-desktop-v1';
    const operations = {
      Start: async () => ({ contract, data: ++starts === 1 ? null : { instance_id: 'local-fixture', profile_name: '@local', transport: 'local', target: '本机', source: 'reused', phase: 'ready', can_stop: true }, problem: starts === 1 ? { code: 'offline' } : null }),
      Providers: async () => ({ contract, data: [], problem: null }),
    };
    window.go = { desktop: { Bridge: new Proxy(operations, { get: (target, key) => target[key as keyof typeof operations] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
  });
  await page.goto('/');
  await expect(page.getByRole('button', { name: '管理设备连接', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重新连接当前设备', exact: true }).click();
  await expect(page.getByRole('button', { name: /本机实例 · 已连接/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '供应商', exact: true })).toBeVisible();
});

for (const selection of ['local', 'legacy', 'ambiguous', 'unreadable']) {
  test(`offline device identity handles ${selection} profiles without guessing`, async ({ page }) => {
    await page.addInitScript(({ fixture, selection }) => {
      const contract = fixture.contract;
      const profiles = fixture.profiles.data.map(row => {
        const profile = { ...row, selected: selection === 'ambiguous' || (selection === 'local' && row.transport === 'local') };
        if (selection === 'legacy') delete (profile as { selected?: boolean }).selected;
        return profile;
      });
      window.go = { desktop: { Bridge: new Proxy({}, { get: (_, key) => async () => key === 'Connections' && selection !== 'unreadable' ? ({ contract, data: profiles, problem: null }) : ({ contract, data: null, problem: { code: 'offline' } }) }) as DesktopBridge } };
    }, { fixture, selection });
    await page.goto('/');
    await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible();
    if (selection === 'local') {
      await expect(page.getByText('连接目标：本机（此电脑）', { exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: /本机实例 · 未连接/ })).toBeVisible();
    } else {
      await expect(page.getByRole('button', { name: /设备待确认 · 未连接/ })).toBeVisible();
      await expect(page.getByText('连接目标尚未确认，请前往设备连接查看或选择设备。', { exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: /本机实例 · 未连接/ })).toHaveCount(0);
    }
    await expect(page.getByRole('button', { name: '恢复本机核心', exact: true })).toHaveCount(0);
  });
}

async function quickSwitchHost(page: import('@playwright/test').Page, authentication = 'key') {
  await page.addInitScript(({ fixture, authentication }) => {
    const contract = fixture.contract;
    let current: SessionInfo = { ...fixture.session.data, profile_name: '@local', instance_id: 'local-fixture', transport: 'local', target: '本机', version: '0.1.0-dev.desktop.58', source: 'reused' };
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
        current = input.name === '@local' ? { ...current, profile_name: '@local', instance_id: 'local-fixture', transport: 'local', target: '本机', version: '0.1.0-dev.desktop.58', source: 'reused' } : fixture.session.data;
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
  await expect(panel).toContainText('核心版本：0.1.0-dev.desktop.58');
  await expect(panel).not.toContainText('local-fixture');
  await expect(panel).not.toContainText('remote-fixture');
  await expect(panel.getByRole('button', { name: '切换到本机', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await panel.getByRole('button', { name: '切换到dev-server' }).click();
  await expect(panel.getByRole('button', { name: '切换到本机', exact: true })).toBeDisabled();
  await expect(panel.getByRole('button', { name: '停止核心', exact: true })).toBeDisabled();
  await expect(panel).toBeHidden();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toContainText(fixture.session.data.version);
  await expect(page.getByRole('heading', { name: '供应商', exact: true })).toBeVisible();
  await page.getByRole('button', { name: /dev-server · 已连接/ }).click();
  await expect(panel.getByRole('button', { name: '切换到dev-server' })).toHaveAttribute('aria-pressed', 'true');
  await expect(panel.getByRole('button', { name: '切换到dev-server' })).toContainText(`核心版本：${fixture.session.data.version}`);
  await expect(panel).not.toContainText('0.1.0-dev.desktop.58');
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

test('remote connection form preserves Chinese preedit and reserves candidate Enter for the IME', async ({ page }) => {
  await quickSwitchHost(page);
  await page.getByRole('button', { name: '设备连接', exact: true }).click();
  const input = page.getByLabel('连接名称');
  await input.fill('');
  await page.getByLabel('SSH 目标').fill('developer@192.0.2.10');
  await page.getByLabel('信任首次连接的主机密钥').check();
  await input.evaluate(node => {
    const form = (node as HTMLInputElement).form!;
    form.dataset.submits = '0';
    form.addEventListener('submit', event => {
      event.preventDefault(); event.stopImmediatePropagation();
      form.dataset.submits = String(Number(form.dataset.submits) + 1);
    });
  });
  await input.focus();
  const session = await page.context().newCDPSession(page);
  await session.send('Input.imeSetComposition', { text: 'shebei ', selectionStart: 7, selectionEnd: 7 });
  await expect(input).toHaveValue('shebei ');
  await session.send('Input.insertText', { text: '中文设备' });
  await expect(input).toHaveValue('中文设备');
  await input.dispatchEvent('compositionstart');
  // Real Enter with isComposing=false models WebViews that require the composition-event fallback.
  await input.press('Enter');
  await expect(page.locator('form')).toHaveAttribute('data-submits', '0');
  await input.dispatchEvent('compositionend');
  await input.press('Enter');
  await expect(page.locator('form')).toHaveAttribute('data-submits', '1');
  // A focus change must not leave the next field in a stale composing state.
  await input.dispatchEvent('compositionstart');
  await page.getByLabel('SSH 目标').focus();
  await page.getByLabel('SSH 目标').press('Enter');
  await expect(page.locator('form')).toHaveAttribute('data-submits', '2');
  await session.detach();
});

test('device password popover does not close or connect while accepting IME candidates', async ({ page }) => {
  await quickSwitchHost(page, 'password');
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  const panel = page.getByRole('dialog', { name: '切换设备' });
  await panel.getByRole('button', { name: '切换到dev-server' }).click();
  const input = panel.getByLabel('dev-server 的 SSH 密码');
  await input.fill('quick-switch-test-password');
  await input.dispatchEvent('compositionstart');
  await input.press('Escape');
  await expect(panel).toBeVisible();
  await expect(input).toHaveValue('quick-switch-test-password');
  await input.press('Enter');
  expect(await page.evaluate(() => (window as unknown as { quickSwitchTest: { calls: unknown[] } }).quickSwitchTest.calls)).toHaveLength(0);
  await input.dispatchEvent('compositionend');
  await input.press('Enter');
  await expect(panel).toBeHidden();
  expect(await page.evaluate(() => (window as unknown as { quickSwitchTest: { calls: unknown[] } }).quickSwitchTest.calls)).toHaveLength(1);
  await page.getByRole('button', { name: /dev-server · 已连接/ }).click();
  await panel.press('Escape');
  await expect(panel).toBeHidden();
});

async function passwordVaultHost(page: import('@playwright/test').Page, created = true) {
  const state = { created, saved: false, current: '', selected: created ? 'dev-server' : '@local', failSave: false, failForget: false, calls: [] as { method: string; input: SelectConnection | SSHConnectionInput | undefined }[] };
  const contract = fixture.contract;
  const local: SessionInfo = { ...fixture.session.data, instance_id: 'local-fixture', profile_name: '@local', transport: 'local', target: '本机', source: 'reused' };
  const profiles = () => ({ contract, data: fixture.profiles.data.filter(row => row.transport === 'local' || state.created).map(row => ({ ...row, active: state.current === row.name, selected: state.selected === row.name, authentication: row.transport === 'ssh' ? 'password' : 'owner', password_saved: row.transport === 'ssh' && state.saved })), problem: null });
  await page.exposeFunction('passwordVaultHost', async (method: string, input?: SelectConnection | SSHConnectionInput) => {
    const failure = (code: string) => ({ contract, data: null, problem: { code } });
    if (method === 'Connections') return profiles();
    if (method === 'Providers') return { contract, data: [], problem: null };
    if (method === 'Status') return { contract, data: state.current === '@local' ? local : fixture.session.data, problem: null };
    if (method === 'Start') {
      if (!state.created) { state.current = '@local'; return { contract, data: local, problem: null }; }
      if (!state.saved) return failure('ssh_auth');
      state.current = 'dev-server'; return fixture.session;
    }
    state.calls.push({ method, input });
    if (method === 'ForgetConnectionPassword') {
      if (state.failForget) return failure('credential_store');
      state.saved = false; return profiles();
    }
    if (method === 'ConnectSSH' || method === 'UseConnection') {
      await new Promise(resolve => setTimeout(resolve, 200));
      if (method === 'UseConnection' && input?.name === '@local') { state.current = state.selected = '@local'; return { contract, data: local, problem: null }; }
      if (input?.password !== 'vault-fixture-password' && !(input?.password === '' && state.saved)) return failure('ssh_auth');
      state.created = true; state.current = state.selected = 'dev-server';
      if (input?.remember_password && !state.failSave) state.saved = true;
      return { ...fixture.session, ...(input?.remember_password && state.failSave ? { warning: { code: 'credential_store' } } : {}) };
    }
    return failure('unsupported');
  });
  await page.addInitScript(() => {
    const host = (window as unknown as { passwordVaultHost: (method: string, input?: unknown) => Promise<unknown> }).passwordVaultHost;
    window.go = { desktop: { Bridge: new Proxy({}, { get: (_, method) => (input: unknown) => host(String(method), input) }) as DesktopBridge } };
  });
  await page.goto('/');
  return state;
}

test('remembered device password survives GUI reload and forgetting keeps the current session', async ({ page }) => {
  const state = await passwordVaultHost(page);
  await page.getByRole('button', { name: '重新连接当前设备', exact: true }).click();
  const device = page.locator('.connection-profile').filter({ hasText: 'dev-server' });
  await device.getByLabel('dev-server 的 SSH 密码').fill('vault-fixture-password');
  const remember = device.getByRole('checkbox', { name: '记住dev-server 本次输入的密码' });
  await expect(remember).not.toBeChecked();
  await remember.check();
  await device.getByRole('button', { name: '连接', exact: true }).click();
  await expect(device.getByText('已记住密码（系统凭据库）', { exact: true })).toBeVisible();
  await expect(device.getByLabel('dev-server 的 SSH 密码')).toHaveValue('');
  expect(state.calls.filter(call => call.method === 'UseConnection')).toHaveLength(1);
  expect(state.calls[0].input).toMatchObject({ remember_password: true });
  await page.reload();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  await page.getByRole('button', { name: /dev-server · 已连接/ }).click();
  await page.getByRole('button', { name: '切换到本机', exact: true }).click();
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await page.getByRole('button', { name: '切换到dev-server', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '切换设备' })).toBeHidden();
  expect(state.calls.at(-1)?.input).toMatchObject({ name: 'dev-server', password: '' });
  await page.getByRole('button', { name: '设备连接', exact: true }).click();
  state.failForget = true;
  await device.getByRole('button', { name: '忘记dev-server 的密码' }).click();
  await expect(page.getByRole('alert')).toContainText('系统凭据库不可用');
  await expect(device.getByText('已记住密码（系统凭据库）', { exact: true })).toBeVisible();
  state.failForget = false;
  await device.getByRole('button', { name: '忘记dev-server 的密码' }).click();
  await expect(page.getByText('已忘记保存的密码。', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('vault-fixture-password');
  await page.reload();
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible();
});

test('new SSH device defaults to one-time password and can opt into system storage', async ({ page }) => {
  const state = await passwordVaultHost(page, false);
  await page.getByRole('button', { name: '设备连接', exact: true }).click();
  await page.getByLabel('连接名称').fill('dev-server');
  await page.getByLabel('SSH 目标').fill('developer@192.0.2.10');
  await page.getByLabel('使用密码登录').check();
  const remember = page.getByRole('checkbox', { name: /记住密码（系统凭据库）/ });
  await expect(remember).not.toBeChecked();
  await page.getByLabel('SSH 密码', { exact: true }).fill('vault-fixture-password');
  await remember.check();
  await page.getByRole('button', { name: '添加并连接', exact: true }).click();
  await expect(page.getByText('已记住密码（系统凭据库）', { exact: true })).toBeVisible();
  expect(state.calls.find(call => call.method === 'ConnectSSH')?.input).toMatchObject({ remember_password: true, password: 'vault-fixture-password' });
  await expect(page.getByLabel('SSH 密码', { exact: true })).toHaveValue('');
});

test('header can remember a password and preserves ready state when system storage fails', async ({ page }) => {
  const state = await passwordVaultHost(page);
  state.failSave = true;
  await page.getByRole('button', { name: /dev-server · 未连接/ }).click();
  const panel = page.getByRole('dialog', { name: '切换设备' });
  await panel.getByRole('button', { name: '切换到dev-server', exact: true }).click();
  await panel.getByLabel('dev-server 的 SSH 密码').fill('vault-fixture-password');
  await panel.getByRole('checkbox', { name: '记住密码（系统凭据库）', exact: true }).check();
  await panel.getByRole('button', { name: '连接设备', exact: true }).click();
  await expect(panel.getByText('设备已连接，但未能确认本次密码已保存。', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: /dev-server · 已连接/ })).toBeVisible();
  await expect(panel.getByLabel('dev-server 的 SSH 密码')).toHaveValue('');
  expect(state.saved).toBe(false);
});
