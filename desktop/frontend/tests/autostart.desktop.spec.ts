import { test, expect, type Page } from '@playwright/test';
import fixture from '../../contract/autostart.json' with { type: 'json' };
import session from '../../contract/core-restart.json' with { type: 'json' };
import type { DesktopBridge, SaveAutostart } from '../src/desktop/contract.generated';

declare global { interface Window { autostartTest: { requests: SaveAutostart[]; reads: number; fail: boolean; hold: boolean; release?: () => void } } }
async function host(page: Page, supported: boolean | 'legacy' = true) {
  await page.addInitScript(({ fixture, session, supported }) => {
    const contract = fixture.contract;
    const state: Window['autostartTest'] = window.autostartTest = { requests: [] as SaveAutostart[], reads: 0, fail: false, hold: false };
    let data = supported ? fixture.data : { ...fixture.data, supported: false, can_enable: false, revision: '' };
    const ops = {
      Start: async () => session.remote_success,
      Status: async () => session.remote_success,
      Connections: async () => ({ contract, data: [], problem: null }),
      Providers: async () => ({ contract, data: [], problem: null }),
      Autostart: async () => { state.reads++; return supported === 'legacy' ? { contract, data: null, problem: { code: 'unsupported' } } : { contract, data, problem: null }; },
      SaveAutostart: async (input: SaveAutostart) => {
        state.requests.push(input);
        if (state.hold) await new Promise<void>(resolve => { state.release = resolve; });
        if (state.fail) return { contract, data: null, problem: { code: 'unknown' } };
        data = { ...data, enabled: input.enabled!, matches_current: input.enabled! };
        return { contract, data, problem: null };
      },
    };
    window.go = { desktop: { Bridge: new Proxy(ops, { get: (target, key) => target[key as keyof typeof ops] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
  }, { fixture, session, supported });
  await page.goto('/');
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await expect(page.getByRole('heading', { name: '本机登录自启' })).toBeVisible();
}

test('local startup remains opt-in even with a remote session selected', async ({ page }) => {
  await host(page);
  const control = page.getByRole('switch', { name: '登录 Windows 后启动后台核心' });
  await expect(control).not.toBeChecked();
  expect(await page.evaluate(() => window.autostartTest.requests)).toEqual([]);
  await control.click();
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('不启动 Codex');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.autostartTest.requests)).toEqual([]);
  await control.click();
  await page.evaluate(() => { window.autostartTest.hold = true; });
  await dialog.getByRole('button', { name: '确认变更' }).click();
  await expect(dialog.getByRole('button', { name: '正在提交…' })).toBeDisabled();
  expect(await page.evaluate(() => window.autostartTest.requests.length)).toBe(1);
  await page.evaluate(() => { window.autostartTest.hold = false; window.autostartTest.release?.(); });
  await expect(control).toBeChecked();
  expect(await page.evaluate(() => window.autostartTest.requests)).toEqual([{ enabled: true, approved: true, expected_revision: fixture.data.revision }]);
  await page.screenshot({ path: 'test-results/autostart-desktop.png', fullPage: true });
  await page.setViewportSize({ width: 760, height: 900 });
  await expect(control).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: 'test-results/autostart-compact.png', fullPage: true });
  await control.click();
  await dialog.getByRole('button', { name: '确认变更' }).click();
  await expect(control).not.toBeChecked();
});

test('unknown writes require a read instead of replay', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.autostartTest.fail = true; });
  const control = page.getByRole('switch', { name: '登录 Windows 后启动后台核心' });
  await control.click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('结果未知，请重新读取后再操作。')).toBeVisible();
  await expect(control).toBeDisabled();
  expect(await page.evaluate(() => window.autostartTest.requests.length)).toBe(1);
  await page.getByRole('button', { name: '重新读取登录项' }).click();
  await expect(control).toBeEnabled();
  expect(await page.evaluate(() => window.autostartTest.requests.length)).toBe(1);
});

test('unsupported hosts never expose a working startup toggle', async ({ page }) => {
  await host(page, false);
  await expect(page.getByText('当前平台不支持', { exact: true })).toBeVisible();
  await expect(page.getByRole('switch', { name: '登录 Windows 后启动后台核心' })).toBeDisabled();
  expect(await page.evaluate(() => window.autostartTest.requests.length)).toBe(0);
});

test('legacy hosts do not add an error to unrelated settings', async ({ page }) => {
  await host(page, 'legacy');
  await expect(page.getByText('当前宿主版本不支持', { exact: true })).toBeVisible();
  const section = page.locator('section').filter({ has: page.getByRole('heading', { name: '本机登录自启' }) });
  await expect(section.getByRole('alert')).toHaveCount(0);
  await expect(section.getByRole('switch')).toBeDisabled();
});
