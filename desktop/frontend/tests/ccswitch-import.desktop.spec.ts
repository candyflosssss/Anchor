import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/ccswitch-import.json' with { type: 'json' };
import type { DesktopBridge, Provider } from '../src/desktop/contract.generated';

async function host(page: Page, failure = '') {
  await page.addInitScript(({ fixture, failure }) => {
    const contract = 'anchor-desktop-v1';
    const session = { instance_id: 'ccs-test', phase: 'ready', source: 'reused', profile_name: '@local', transport: 'local', target: '本机', can_stop: true };
    const rows: Provider[] = [];
    let writes = 0;
    const api: Partial<DesktopBridge> = {
      Start: async () => ({ contract, data: session, problem: null }),
      Status: async () => ({ contract, data: session, problem: null }),
      Providers: async () => ({ contract, data: rows, problem: null }),
      ImportCCSwitch: async input => {
        if (!input.approved) return { contract, data: { ...fixture, providers: fixture.providers.map(p => ({ ...p, status: writes && failure !== 'conflict' ? 'exists' : 'ready' })) }, problem: null };
        writes++;
        if (input.fingerprint !== fixture.fingerprint || failure === 'conflict') return { contract, data: null, problem: { code: 'conflict' } };
        if (writes > 1) throw new Error('duplicate write');
        rows.push({ id: 'ccs-demo', homepage_url: '', base_urls: fixture.providers[0].base_urls, protocol: 'responses', enabled: true, cost_multiplier: 1, timeout_seconds: 120, max_concurrent: 4, revision: 1, has_secret: true });
        if (failure === 'unknown') throw new Error('lost response');
        return { contract, data: { ...fixture, applied: true, imported_providers: 1, imported_keys: 2, imported_models: 1 }, problem: null };
      },
    };
    // Unrelated optional workspaces stay explicitly unavailable in this fake.
    window.go = { desktop: { Bridge: new Proxy(api, { get: (target, key) => target[key as keyof DesktopBridge] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as DesktopBridge } };
  }, { fixture, failure });
  await page.goto('/');
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await page.getByRole('button', { name: '从 CC Switch 导入', exact: true }).click();
}

test('reviews all platforms, applies once and refreshes the provider list', async ({ page }) => {
  await host(page);
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('将启用 1 个供应商、2 个 Key');
  await expect(dialog).toContainText('claude / 演示供应商、codex / 演示供应商');
  await expect(dialog).toContainText('Gemini 演示：此配置的上游协议尚不受 Anchor 支持');
  await page.getByRole('button', { name: '确认导入并启用', exact: true }).evaluate(button => { (button as HTMLButtonElement).click(); (button as HTMLButtonElement).click(); });
  await expect(dialog).toContainText('已启用 1 个供应商、2 个 Key');
  await expect(page.getByRole('button', { name: '确认导入并启用', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByRole('button', { name: 'ccs-demo', exact: true })).toBeVisible();
});

for (const failure of ['conflict', 'unknown']) test(`${failure} requires a fresh preview before another write`, async ({ page }) => {
  await host(page, failure);
  await page.getByRole('button', { name: '确认导入并启用', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText(failure === 'conflict' ? '配置已变化' : '导入结果未知');
  await expect(page.getByRole('button', { name: '确认导入并启用', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '读取并预览', exact: true }).click();
  if (failure === 'unknown') {
    await expect(page.getByRole('dialog')).toContainText('已存在，保留现有配置');
    await expect(page.getByRole('button', { name: '确认导入并启用', exact: true })).toBeDisabled();
  } else await expect(page.getByRole('button', { name: '确认导入并启用', exact: true })).toBeEnabled();
});
