import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import account from '../../contract/provider-account.json' with { type: 'json' };
import update from '../../contract/updates.json' with { type: 'json' };
import { CONTRACT, type AccountQuery, type SaveProvider } from '../src/desktop/contract.generated';

declare global { interface Window { balanceTest: { queries: AccountQuery[]; active: number; peak: number; slow: boolean; hold: boolean; failure: string; value: string; revision: number; platform: string; instance: string } } }
async function host(page: Page) {
  await page.addInitScript(({ contract, provider, account, update }) => {
    const counters = window.balanceTest = { queries: [] as AccountQuery[], active: 0, peak: 0, slow: false, hold: false, failure: '', value: '12.34567890123456789', revision: 1, platform: '', instance: 'isolated-balance-test' };
    const rows = ['wallet', 'quota', 'third', 'fourth'].map(id => ({ ...provider, id, revision: 1 }));
    const revision = (id: string) => Math.max(rows.find(p => p.id === id)?.revision ?? 1, counters.revision);
    const settings = (id: string) => ({ ...account.settings, provider_id: id, platform: '', revision: revision(id) });
    const ready = { contract, data: { instance_id: 'isolated-balance-test', phase: 'ready', source: 'started', can_stop: true }, problem: null };
    const api = {
      Status: async () => ({ ...ready, data: { ...ready.data, instance_id: counters.instance } }), Start: async () => ready,
      Providers: async () => ({ contract, data: rows.map(p => ({ ...p, revision: revision(p.id) })), problem: null }),
      SaveProvider: async (q: SaveProvider) => {
        const at = rows.findIndex(p => p.id === q.provider.id);
        if (at < 0 || q.expected_revision !== revision(q.provider.id)) return { contract, data: null, problem: { code: 'conflict' } };
        rows[at] = { ...rows[at], ...q.provider, revision: q.expected_revision + 1 };
        return { contract, data: rows[at], problem: null };
      },
      ProviderWorkspace: async (q: { provider_id: string }) => ({ contract, data: { provider_id: q.provider_id, revision: revision(q.provider_id), strategy: 'round-robin', keys: [], models: [], prices: [], estimates: [], imported_price: null, usage: [], probe: null, deleted: false }, problem: null }),
      ProviderAccountSettings: async (q: { provider_id: string }) => ({ contract, data: settings(q.provider_id), problem: null }),
      ProviderAllowance: async (q: AccountQuery) => {
        const value = counters.value, failure = counters.failure, delay = counters.slow ? 700 : 60;
        counters.queries.push(q); counters.active++; counters.peak = Math.max(counters.peak, counters.active);
        if (counters.hold) await new Promise<void>(resolve => window.addEventListener('release-balance-queries', () => resolve(), { once: true }));
        await new Promise(resolve => setTimeout(resolve, delay)); counters.active--;
        if (failure === 'transport') throw new Error('isolated transport failure');
        const source = q.provider_id === 'quota' ? account.key_summary : account.summary;
        return { contract, data: { ...source, provider_id: q.provider_id, revision: q.expected_revision, platform: counters.platform || source.platform, operation: q.operation, key_id: 'default', remote_key_id: q.remote_key_id, failure,
          allowance: failure ? null : { ...source.allowance, scope: q.operation === 'balance' ? 'account' : q.operation === 'key-quota' ? 'key' : source.allowance.scope, remaining: { value, unit: source.allowance.remaining.unit } } }, problem: null };
      },
      UpdateStatus: async () => ({ contract, data: { ...update, available: false, phase: 'idle' }, problem: null }),
      AgentWorkspace: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
    };
    Object.assign(window, { go: { desktop: { Bridge: api } } });
  }, { contract: CONTRACT, provider: fixture.providers[0], account, update: update.downloaded });
}

test('supplier list shows scoped balances, detects platform and refreshes with default Key', async ({ page }) => {
  await host(page); await page.goto('/');
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  const quota = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'quota', exact: true }) });
  await expect(wallet).toContainText('12.35 USD');
  await expect(wallet).toContainText('sub2api · 账户余额');
  await expect(quota).toContainText('NewAPI · Key 额度');
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  const counters = await page.evaluate(() => window.balanceTest);
  expect(counters.queries).toHaveLength(4); expect(counters.peak).toBeLessThanOrEqual(2);
  expect(counters.queries.every(q => q.key_id === '' && q.remote_key_id === 0 && q.operation === 'balance-summary')).toBe(true);
  await page.evaluate(() => { window.balanceTest.failure = 'auth_required'; });
  await page.getByRole('button', { name: '刷新供应商列表' }).click();
  await expect(wallet).toContainText('余额未刷新'); await expect(wallet).toContainText('12.35 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  const beforeRetry = await page.evaluate(() => window.balanceTest.queries.length);
  await page.evaluate(() => { window.balanceTest.failure = ''; window.balanceTest.value = '23'; });
  await wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true }).click();
  await expect(wallet).toContainText('23.00 USD'); await expect(wallet).not.toContainText('余额未刷新');
  expect(await page.evaluate(() => window.balanceTest.queries.length)).toBe(beforeRetry + 1);
  await wallet.getByRole('button', { name: 'wallet', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额' }).click();
  await expect(page.locator('#account-platform')).toHaveValue('auto');
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeEnabled();
});

test('provider toggles keep cached balances and manual refresh uses the latest revision', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  const toggle = wallet.getByRole('switch', { name: '供应商 wallet 启用状态' });
  await toggle.click(); await expect(toggle).not.toBeChecked();
  await expect(wallet).toContainText('12.35 USD');
  await toggle.click(); await expect(toggle).toBeChecked();
  // Observe a quiet period longer than the simulated bridge query delay.
  await page.waitForTimeout(200);
  expect(await page.evaluate(() => window.balanceTest.queries.length)).toBe(4);
  await page.evaluate(() => { window.balanceTest.value = '23'; });
  await wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true }).click();
  await expect(wallet).toContainText('23.00 USD');
  const queries = await page.evaluate(() => window.balanceTest.queries);
  expect(queries).toHaveLength(5);
  expect(queries.at(-1)?.expected_revision).toBe(3);
});

for (const id of ['wallet', 'third']) test(`toggling ${id} does not restart pending or queued balance reads`, async ({ page }) => {
  await host(page); await page.goto('/');
  await page.getByRole('button', { name: '概览', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  await page.evaluate(() => { window.balanceTest.queries = []; window.balanceTest.hold = true; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  const row = page.getByRole('row').filter({ has: page.getByRole('button', { name: id, exact: true }) });
  const toggle = row.getByRole('switch', { name: `供应商 ${id} 启用状态` });
  await toggle.click(); await expect(toggle).not.toBeChecked();
  await page.evaluate(() => { window.balanceTest.hold = false; window.dispatchEvent(new Event('release-balance-queries')); });
  await expect(row).toContainText('12.35 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  const queries = await page.evaluate(() => window.balanceTest.queries);
  expect(queries).toHaveLength(4);
  expect(queries.filter(q => q.provider_id === id)).toHaveLength(1);
  if (id === 'third') expect(queries.find(q => q.provider_id === id)?.expected_revision).toBe(2);
});

test('late results from an old revision cannot replace refreshed balances', async ({ page }) => {
  await host(page); await page.goto('/');
  await page.evaluate(() => { window.balanceTest.slow = true; window.balanceTest.value = '111'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  await page.evaluate(() => { window.balanceTest.slow = false; window.balanceTest.value = '222'; window.balanceTest.revision = 2; });
  await page.getByRole('button', { name: '刷新供应商列表' }).click();
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(wallet).toContainText('222.00 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  await expect(wallet).toContainText('222.00 USD'); await expect(wallet).not.toContainText('111');
});

test('automatically identified OneAPI uses the supplied remote Key ID', async ({ page }) => {
  await host(page); await page.goto('/');
  await page.evaluate(() => { window.balanceTest.platform = 'oneapi'; window.balanceTest.failure = 'auth_required'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await page.getByRole('button', { name: 'wallet', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额' }).click();
  await page.getByRole('button', { name: '刷新余额', exact: true }).click();
  await expect(page.locator('#account-remote-id')).toBeVisible();
  await page.locator('#account-remote-id').fill('42');
  await page.evaluate(() => { window.balanceTest.failure = ''; });
  await page.getByRole('button', { name: '刷新额度', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.queries.at(-1)?.remote_key_id)).toBe(42);
  const quota = page.locator('.detail-card').filter({ has: page.getByRole('heading', { name: 'Key 额度', exact: true }) });
  await expect(quota).toContainText('12.35 USD');
});


test('navigation shows cached balances while revalidating, including after settings unmounts the page', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await page.getByRole('button', { name: '概览', exact: true }).click();
  await page.evaluate(() => { window.balanceTest.hold = true; window.balanceTest.value = '222'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(wallet).toContainText('12.35 USD');
  await expect(wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true })).toBeDisabled();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  await expect(wallet).not.toContainText('查询中');
  await page.evaluate(() => { window.balanceTest.hold = false; window.dispatchEvent(new Event('release-balance-queries')); });
  await expect(wallet).toContainText('222.00 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await page.evaluate(() => { window.balanceTest.hold = true; window.balanceTest.value = '333'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(wallet).toContainText('222.00 USD');
  await expect(wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true })).toBeDisabled();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  await expect(wallet).not.toContainText('查询中');
  await page.evaluate(() => { window.balanceTest.hold = false; window.dispatchEvent(new Event('release-balance-queries')); });
  await expect(wallet).toContainText('333.00 USD');
});

test('transport failures keep cached balances and a manual retry shares the bounded refresh queue', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await page.evaluate(() => { window.balanceTest.failure = 'transport'; });
  await wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true }).click();
  await expect(wallet).toContainText('余额未刷新');
  await expect(wallet).toContainText('12.35 USD');
  await page.getByRole('button', { name: '概览', exact: true }).click();
  await page.evaluate(() => { window.balanceTest.failure = ''; window.balanceTest.value = '444'; window.balanceTest.slow = true; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(wallet).toContainText('余额未刷新');
  await expect(wallet).toContainText('444.00 USD');
  await expect(wallet).not.toContainText('余额未刷新');
  // The other two providers are still queued/running when this row finishes.
  await wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true }).click();
  await expect(wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true })).toBeDisabled();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  expect(await page.evaluate(() => window.balanceTest.peak)).toBeLessThanOrEqual(2);
});

test('configuration revisions invalidate cached balances', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await page.evaluate(() => { window.balanceTest.revision = 2; window.balanceTest.failure = 'network'; });
  await page.getByRole('button', { name: '刷新供应商列表' }).click();
  await expect(wallet).toContainText('余额未刷新');
  await expect(wallet).not.toContainText('12.34567890123456789');
  await expect(wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true })).toBeEnabled();
});

test('switching the connected instance discards cached balances', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await page.evaluate(() => { window.balanceTest.instance = 'another-isolated-instance'; window.balanceTest.failure = 'network'; });
  await expect(wallet).toContainText('余额未刷新', { timeout: 8000 });
  await expect(wallet).not.toContainText('12.34567890123456789');
});


test('cancelled queries cannot overwrite the cache after the same instance page remounts', async ({ page }) => {
  await host(page); await page.goto('/');
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  await page.evaluate(() => { window.balanceTest.hold = true; window.balanceTest.value = '111'; });
  await wallet.getByRole('button', { name: '刷新 wallet 的余额', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(1);
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await page.evaluate(() => { window.balanceTest.hold = false; window.balanceTest.value = '222'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(wallet).toContainText('222.00 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(1);
  await page.evaluate(() => { window.dispatchEvent(new Event('release-balance-queries')); });
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  await expect(wallet).toContainText('222.00 USD');
  await page.getByRole('button', { name: '概览', exact: true }).click();
  await page.evaluate(() => { window.balanceTest.hold = true; window.balanceTest.value = '333'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  await expect(wallet).toContainText('222.00 USD');
  await expect(wallet).not.toContainText('111.00 USD');
  await page.evaluate(() => { window.balanceTest.hold = false; window.dispatchEvent(new Event('release-balance-queries')); });
  await expect(wallet).toContainText('333.00 USD');
});
