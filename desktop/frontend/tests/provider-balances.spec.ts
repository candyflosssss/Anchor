import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import account from '../../contract/provider-account.json' with { type: 'json' };
import update from '../../contract/updates.json' with { type: 'json' };
import { CONTRACT, type AccountQuery } from '../src/desktop/contract.generated';

declare global { interface Window { balanceTest: { queries: AccountQuery[]; active: number; peak: number; slow: boolean; failure: string; value: string; revision: number; platform: string } } }
async function host(page: Page) {
  await page.addInitScript(({ contract, provider, account, update }) => {
    const counters = window.balanceTest = { queries: [] as AccountQuery[], active: 0, peak: 0, slow: false, failure: '', value: '12.34567890123456789', revision: 1, platform: '' };
    const rows = ['wallet', 'quota', 'third', 'fourth'].map(id => ({ ...provider, id, revision: 1 }));
    const settings = (id: string) => ({ ...account.settings, provider_id: id, platform: '', revision: counters.revision });
    const ready = { contract, data: { instance_id: 'isolated-balance-test', phase: 'ready', source: 'started', can_stop: true }, problem: null };
    const api = {
      Status: async () => ready, Start: async () => ready,
      Providers: async () => ({ contract, data: rows.map(p => ({ ...p, revision: counters.revision })), problem: null }),
      ProviderWorkspace: async (q: { provider_id: string }) => ({ contract, data: { provider_id: q.provider_id, revision: counters.revision, strategy: 'round-robin', keys: [], models: [], prices: [], estimates: [], imported_price: null, usage: [], probe: null, deleted: false }, problem: null }),
      ProviderAccountSettings: async (q: { provider_id: string }) => ({ contract, data: settings(q.provider_id), problem: null }),
      ProviderAllowance: async (q: AccountQuery) => {
        const value = counters.value, failure = counters.failure, delay = counters.slow ? 700 : 60;
        counters.queries.push(q); counters.active++; counters.peak = Math.max(counters.peak, counters.active);
        await new Promise(resolve => setTimeout(resolve, delay)); counters.active--;
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
  await expect(wallet).toContainText('12.34567890123456789 USD');
  await expect(wallet).toContainText('sub2api · 账户余额');
  await expect(quota).toContainText('NewAPI · Key 额度');
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: 'fourth', exact: true }) })).toContainText('USD');
  const counters = await page.evaluate(() => window.balanceTest);
  expect(counters.queries).toHaveLength(4); expect(counters.peak).toBeLessThanOrEqual(2);
  expect(counters.queries.every(q => q.key_id === '' && q.remote_key_id === 0 && q.operation === 'balance-summary')).toBe(true);
  await page.evaluate(() => { window.balanceTest.failure = 'auth_required'; });
  await page.getByRole('button', { name: '刷新供应商列表' }).click();
  await expect(wallet).toContainText('需额外授权'); await expect(wallet).not.toContainText('12.34567890123456789');
  await wallet.getByRole('button', { name: 'wallet', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额' }).click();
  await expect(page.locator('#account-platform')).toHaveValue('auto');
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeEnabled();
});

test('late results from an old revision cannot replace refreshed balances', async ({ page }) => {
  await host(page); await page.goto('/');
  await page.evaluate(() => { window.balanceTest.slow = true; window.balanceTest.value = '111'; });
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(2);
  await page.evaluate(() => { window.balanceTest.slow = false; window.balanceTest.value = '222'; window.balanceTest.revision = 2; });
  await page.getByRole('button', { name: '刷新供应商列表' }).click();
  const wallet = page.getByRole('row').filter({ has: page.getByRole('button', { name: 'wallet', exact: true }) });
  await expect(wallet).toContainText('222 USD');
  await expect.poll(() => page.evaluate(() => window.balanceTest.active)).toBe(0);
  await expect(wallet).toContainText('222 USD'); await expect(wallet).not.toContainText('111');
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
  await expect(quota).toContainText('12.34567890123456789 USD');
});
