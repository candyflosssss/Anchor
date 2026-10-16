import { test, expect, type Page } from '@playwright/test';
import fixture from '../../contract/core-restart.json' with { type: 'json' };
import type { ConfirmStop, DesktopBridge, SessionResult } from '../src/desktop/contract.generated';

declare global { interface Window { restartTest: {
  starts: number; requests: ConfirmStop[]; offline: boolean; remote: boolean;
  failure: 'unknown' | 'startup_failed' | null; pendingStatus: boolean;
  deferStatus: boolean; releaseStatus?: () => void; holdRestart?: boolean; releaseRestart?: () => void;
} } }
async function host(page: Page, remote = false) {
  await page.addInitScript(({ fixture, remote }) => {
    const contract = fixture.contract;
    const state = window.restartTest = { starts: 0, requests: [] as ConfirmStop[], offline: false, remote, failure: null, pendingStatus: false, deferStatus: false } as Window['restartTest'];
    const ready = (): SessionResult => ({ ...fixture.success, data: state.remote ? { ...fixture.success.data, profile_name: 'remote-device', transport: 'ssh', source: 'remote', target: 'fixture@192.0.2.1' } : fixture.success.data });
    const offline = (): SessionResult => ({ contract, data: null, problem: { code: 'offline' } });
    const operations = {
      Start: async () => { state.starts++; return ready(); },
      Status: async () => {
        if (state.deferStatus) {
          state.pendingStatus = true;
          return new Promise<SessionResult>(resolve => { state.releaseStatus = () => { state.deferStatus = false; resolve(offline()); }; });
        }
        return state.offline ? offline() : ready();
      },
      RestartCore: async (input: ConfirmStop): Promise<SessionResult> => {
        state.requests.push(input);
        if (state.holdRestart) await new Promise<void>(resolve => { state.releaseRestart = resolve; });
        await new Promise(resolve => setTimeout(resolve, 400));
        if (state.failure) return { contract, data: null, problem: { code: state.failure } };
        state.offline = false;
        return ready();
      },
      Providers: async () => ({ contract, data: [], problem: null }),
      Connections: async () => ({ contract, data: [], problem: null }),
    };
    window.go = { desktop: { Bridge: new Proxy(operations, { get: (target, key) => target[key as keyof typeof operations] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
  }, { fixture, remote });
  await page.goto('/');
  await expect(page.getByRole('button', { name: /· 已连接/ })).toBeVisible();
}
async function openRestart(page: Page) {
  await page.getByRole('button', { name: /· 已连接/ }).click();
  await page.getByRole('button', { name: '重启核心', exact: true }).click();
}

test('restart is confirmed, single-flight, and reports actual readiness', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.restartTest.holdRestart = true; });
  await openRestart(page);
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('fixture-local');
  await expect(dialog).toContainText('保留托管配置');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.restartTest.requests)).toEqual([]);
  await openRestart(page);
  await dialog.getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('button', { name: '正在提交…' })).toBeDisabled();
  await expect(dialog.getByRole('status')).toHaveText('正在重启核心，请稍候…');
  await page.evaluate(() => window.restartTest.releaseRestart?.());
  await expect(page.getByText('核心已恢复，服务已就绪。', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.restartTest.requests)).toEqual([fixture.request]);
});

test('offline local target can be recovered without automatic restart', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.restartTest.offline = true; });
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible({ timeout: 8000 });
  expect(await page.evaluate(() => window.restartTest.requests.length)).toBe(0);
  await page.getByRole('button', { name: '恢复本机核心', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('核心已恢复，服务已就绪。', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.restartTest.requests)).toEqual([fixture.request]);
});

test('unknown restart is checked read-only instead of replayed', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.restartTest.failure = 'unknown'; });
  await openRestart(page);
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('操作结果未知。请先检查核心状态，不要重复提交。', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '恢复本机核心', exact: true })).toBeDisabled();
  const starts = await page.evaluate(() => window.restartTest.starts);
  await page.getByRole('button', { name: '检查核心状态', exact: true }).click();
  await expect(page.getByRole('button', { name: /本机实例 · 已连接/ })).toBeVisible();
  expect(await page.evaluate(() => window.restartTest.starts)).toBe(starts);
  expect(await page.evaluate(() => window.restartTest.requests.length)).toBe(1);
});

test('old polling failures cannot overwrite successful restart', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.restartTest.deferStatus = true; });
  await expect.poll(() => page.evaluate(() => window.restartTest.pendingStatus), { timeout: 8000 }).toBe(true);
  await openRestart(page);
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByText('核心已恢复，服务已就绪。', { exact: true })).toBeVisible();
  await page.evaluate(() => window.restartTest.releaseStatus?.());
  await expect(page.getByRole('button', { name: /本机实例 · 已连接/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toHaveCount(0);
});

test('offline remote retains its label and never offers local recovery', async ({ page }) => {
  await host(page, true);
  await page.getByRole('button', { name: /remote-device · 已连接/ }).click();
  await expect(page.getByRole('button', { name: '重启核心', exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await page.evaluate(() => { window.restartTest.offline = true; });
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible({ timeout: 8000 });
  await expect(page.getByRole('button', { name: /remote-device · 未连接/ })).toBeVisible();
  await expect(page.getByRole('button', { name: '恢复本机核心', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.restartTest.requests.length)).toBe(0);
});
