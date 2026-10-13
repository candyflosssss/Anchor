import accountFixture from '../../contract/provider-account.json' with { type: 'json' };
import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import updateFixture from '../../contract/updates.json' with { type: 'json' };
import { CONTRACT, type AccountSettings, type DesktopBridge, type Provider } from '../src/desktop/contract.generated';

declare global { interface Window { desktopTest: { directories: number; starts: number; saves: number; stops: number; status: boolean; writeError: 'conflict' | 'unknown' | null; checks: number; downloads: number; installs: number; updateError: boolean } } }
declare global { interface Window { accountTest: { saves: number; queries: number; failure: string } } }
async function host(page: Page, rateMappings = false) {
  // A fake Wails boundary exercises the real frontend mode, not a native window.
  await page.addInitScript(({ contract, rows, update, account, rateMappings }) => {
    window.desktopTest = { directories: 0, starts: 0, saves: 0, stops: 0, status: true, writeError: null, checks: 0, downloads: 0, installs: 0, updateError: false };
    const counters = window.desktopTest;
    window.accountTest = { saves: 0, queries: 0, failure: '' };
    const accounts: Record<string, AccountSettings> = {};
    const settings = (id: string): AccountSettings => ({ ...(accounts[id] ?? { ...account.settings, platform: '' }), provider_id: id, revision: providers.find(p => p.id === id)?.revision ?? 1 });
    let providers: Provider[] = rows;
    const data = { instance_id: 'test-local-instance', phase: 'ready', source: 'started', can_stop: true };
    const api: DesktopBridge = {
      ProviderAccountSettings: async input => ({ contract, data: settings(input.provider_id), problem: null }),
      SaveProviderAccount: async input => {
        window.accountTest.saves++;
        const p = providers.find(p => p.id === input.provider_id);
        if (!p || p.revision !== input.expected_revision || counters.writeError === 'conflict') return { contract, data: null, problem: { code: 'conflict' } };
        accounts[p.id] = { provider_id: p.id, revision: ++p.revision, platform: input.platform, site_url: input.site_url, user_id: input.user_id, has_user_token: input.user_token === null ? settings(p.id).has_user_token : !!input.user_token };
        if (counters.writeError === 'unknown') return { contract, data: null, problem: { code: 'unknown' } };
        return { contract, data: settings(p.id), problem: null };
      },
      ProviderRate: async input => {
        window.accountTest.queries++;
        await new Promise(resolve => setTimeout(resolve, 150));
        const failure = window.accountTest.failure;
        return { contract, data: { ...account.rate, provider_id: input.provider_id, revision: input.expected_revision, platform: settings(input.provider_id).platform || 'sub2api', key_id: input.key_id || 'default', remote_key_id: input.remote_key_id, failure,
          rate: failure ? null : { ...account.rate.rate } }, problem: null };
      },
      ProviderAllowance: async input => {
        window.accountTest.queries++;
        const failure = window.accountTest.failure;
        return { contract, data: { ...account.balance, provider_id: input.provider_id, revision: input.expected_revision, platform: settings(input.provider_id).platform, operation: input.operation, key_id: 'anonymous', remote_key_id: input.remote_key_id, failure,
          allowance: failure ? null : { ...account.balance.allowance, scope: input.operation === 'balance' ? 'account' : 'key', remaining: { value: '9007199254740993123', unit: 'quota' } } }, problem: null };
      },
      AgentWorkspace: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      EnrollAgent: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ManageAgent: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      Logs: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogDetail: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogSettings: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ConfigureLogs: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ExportLogs: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      UpdateStatus: async () => ({ contract, data: { ...update, latest_version: '', available: false, phase: 'idle', download_path: '', install_kind: '' }, problem: null }),
      CheckUpdate: async channel => { counters.checks++; return { contract, data: { ...update, channel, phase: 'checked', download_path: '' }, problem: null }; },
      DownloadUpdate: async () => {
        counters.downloads++;
        await new Promise(resolve => setTimeout(resolve, 300));
        return counters.updateError ? { contract, data: null, problem: { code: 'integrity' } } : { contract, data: update, problem: null };
      },
      InstallUpdate: async input => {
        if (!input.approved || input.version !== update.latest_version) return { contract, data: null, problem: { code: 'invalid' } };
        counters.installs++; return { contract, data: { ...update, phase: 'installing' }, problem: null };
      },
      Start: async () => { counters.starts++; return { contract, data, problem: null }; },
      Status: async () => ({ contract, data: counters.status ? data : null, problem: counters.status ? null : { code: 'offline' } }),
      DiscoverProviderModels: async input => {
        counters.directories++;
        await new Promise(resolve => setTimeout(resolve, 300));
        const provider = providers.find(p => p.id === input.provider_id);
        if (provider?.revision !== input.expected_revision) return { contract, data: null, problem: { code: 'conflict' } };
        return { contract, data: { provider_id: provider.id, key_id: input.key_id || 'anonymous', revision: provider.revision, http_status: 200, elapsed_ms: 42, models: ['demo-model'], failure: '', partial: false, entries: [{ id: 'demo-model', key_ids: [input.key_id || 'anonymous'] }], keys: [{ id: input.key_id || 'anonymous', enabled: true, protocol: 'responses', http_status: 200, failure: '', group: '', group_name: '', multiplier: null, rate_basis: '', rate_failure: 'platform_required' }] }, problem: null };
      },
      ProviderWorkspace: async input => ({ contract, data: { provider_id: input.provider_id, revision: providers.find(p => p.id === input.provider_id)?.revision ?? 1, strategy: 'round-robin', prices: [], estimates: [], imported_price: null, keys: [], models: rateMappings ? ['shared-one', 'shared-two'].map(id => ({ id, upstream: `upstream-${id}`, display_name: id, revision: 1, candidate_count: 3, multiplier: input.provider_id === '摸鱼AI' ? 2.5 : 1.2, strategy: 'round-robin', tools: true, streaming: true })) : [], usage: [], probe: null, deleted: false }, problem: null }),
      ProviderAction: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      Providers: async () => ({ contract, data: providers, problem: null }),
      SaveProvider: async input => {
        counters.saves++;
        if (counters.writeError) return { contract, data: null, problem: { code: counters.writeError } };
        const saved = { ...input.provider, revision: input.expected_revision + 1, has_secret: input.secret === null ? input.provider.has_secret : !!input.secret };
        providers = [...providers.filter(p => p.id !== saved.id), saved];
        return { contract, data: saved, problem: null };
      },
      ExitDesktop: async () => ({ contract: CONTRACT, data: { accepted: true }, problem: null }),
      StopCore: async input => {
        if (!input.approved || input.instance_id !== data.instance_id) return { contract, data: null, problem: { code: 'invalid' } };
        counters.stops++; return { contract, data: { accepted: true }, problem: null };
      },
    };
    window.go = { desktop: { Bridge: api } };
  }, { contract: CONTRACT, rows: fixture.providers, update: updateFixture.downloaded, account: accountFixture, rateMappings });
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
}
test('desktop browser without Wails bindings fails closed instead of showing mock data', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible();
  await expect(page.getByText('桌面接口不可用或版本不兼容。', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toHaveCount(0);
  await expect(page.getByLabel('原型场景')).toHaveCount(0);
});
test('desktop starts once, loads real-mode data and submits without persisting secrets', async ({ page }) => {
  await host(page);
  expect(await page.evaluate(() => window.desktopTest.starts)).toBe(1);
  await expect(page.getByText('原型场景', { exact: true })).toHaveCount(0);
  await expect(page.getByText('本机实例 · 模拟，未连接', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await expect(page.getByText('本次启动', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  await page.getByLabel('供应商名称 / ID').fill('桌面供应商');
  await page.getByLabel('API 根地址', { exact: true }).fill('https://desktop.example.com/v1');
  await page.getByRole('dialog').getByRole('button', { name: '添加供应商', exact: true }).dblclick();
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByLabel('桌面供应商 的详情')).toBeVisible();
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('desktop-synthetic-secret');
  await expect(page.getByText('演示配置已保存', { exact: false })).toHaveCount(0);
});
test('stop is separately confirmed, targeted and only reports acceptance', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await page.getByRole('button', { name: '停止核心', exact: true }).click();
  const confirmation = page.getByRole('alertdialog');
  await expect(confirmation).toContainText('test-local-instance');
  await expect(confirmation).toContainText('本次由桌面宿主启动');
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.desktopTest.stops)).toBe(0);
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await page.getByRole('button', { name: '停止核心', exact: true }).click();
  await confirmation.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByRole('heading', { name: '停服请求已接受' })).toBeVisible();
  await expect(page.getByText('核心正在处理退出与配置恢复。', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => ({ stops: window.desktopTest.stops, starts: window.desktopTest.starts }))).toEqual({ stops: 1, starts: 1 });
});
test('disconnect clears open secret drafts and never silently restarts the core', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '编辑 摸鱼AI', exact: true }).click();
  await page.getByRole('button', { name: '替换密钥', exact: true }).click();
  await page.getByLabel('新的 API 密钥').fill('disconnect-synthetic-secret');
  await page.evaluate(() => { window.desktopTest.status = false; });
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible({ timeout: 8000 });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(await page.evaluate(() => ({ starts: window.desktopTest.starts, saves: window.desktopTest.saves }))).toEqual({ starts: 1, saves: 0 });
  expect(await page.content()).not.toContain('disconnect-synthetic-secret');
});
test('a structured write conflict does not replay or claim success', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.desktopTest.writeError = 'conflict'; });
  await page.getByRole('button', { name: '编辑 摸鱼AI', exact: true }).click();
  await page.getByLabel('最大并发').fill('6');
  await page.getByRole('button', { name: '保存配置', exact: true }).click();
  await expect(page.getByRole('dialog').getByText('配置已被其他操作修改。未覆盖新版本；请重新读取后再编辑。')).toBeVisible();
  await expect(page.getByRole('button', { name: '保存配置', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
});

test('polling refreshes the authenticated source and stop capability without restarting', async ({ page }) => {
  await host(page);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!;
    const previous = api.Status;
    api.Status = async () => {
      const result = await previous();
      return { ...result, data: result.data ? { ...result.data, source: 'reused', can_stop: false } : null };
    };
  });
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await expect(page.getByText('复用已有核心', { exact: true })).toBeVisible({ timeout: 8000 });
  await expect(page.getByRole('button', { name: '停止核心', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.desktopTest.starts)).toBe(1);
});

test('online updates require a verified download and explicit confirmation (simulated host only)', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await expect(page.getByRole('heading', { name: '在线更新', exact: true })).toBeVisible();
  await page.getByLabel('更新渠道').selectOption('dev');
  await page.getByRole('button', { name: '检查更新', exact: true }).click();
  await expect(page.getByRole('heading', { name: '发现新版本 0.1.0-dev.desktop.10' })).toBeVisible();
  await page.getByRole('button', { name: '下载更新', exact: true }).dblclick();
  await expect(page.getByRole('heading', { name: '安装包已下载并通过 SHA256 校验' })).toBeVisible();
  expect(await page.evaluate(() => window.desktopTest.downloads)).toBe(1);
  expect(await page.evaluate(() => window.desktopTest.installs)).toBe(0);
  await page.getByRole('button', { name: '安装更新…', exact: true }).click();
  await expect(page.getByRole('alertdialog')).toContainText('正常停止本机核心');
  await page.getByRole('button', { name: '取消', exact: true }).click();
  expect(await page.evaluate(() => window.desktopTest.installs)).toBe(0);
  await page.getByRole('button', { name: '安装更新…', exact: true }).click();
  await page.getByRole('button', { name: '确认变更', exact: true }).click();
  await expect(page.getByRole('heading', { name: '正在交接安装器' })).toBeVisible();
  expect(await page.evaluate(() => window.desktopTest.installs)).toBe(1);
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: '供应商', exact: true })).toBeDisabled();
});
test('an integrity failure never offers installation', async ({ page }) => {
  await host(page);
  await page.evaluate(() => { window.desktopTest.updateError = true; });
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await page.getByRole('button', { name: '检查更新', exact: true }).click();
  await page.getByRole('button', { name: '下载更新', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('SHA256 校验失败');
  await expect(page.getByRole('button', { name: '安装更新…', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.desktopTest.installs)).toBe(0);
});


test('initial update status cannot overwrite a selected channel', async ({ page }) => {
  await host(page);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!;
    const original = api.UpdateStatus;
    api.UpdateStatus = async () => {
      await new Promise(resolve => setTimeout(resolve, 400));
      return original();
    };
  });
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await expect(page.getByLabel('更新渠道')).toBeDisabled();
  await expect(page.getByRole('button', { name: '检查更新', exact: true })).toBeEnabled();
  await page.getByLabel('更新渠道').selectOption('beta');
  await page.getByRole('button', { name: '检查更新', exact: true }).click();
  await expect(page.getByLabel('更新渠道')).toHaveValue('beta');
});

test('changing channels clears the previously downloaded installation candidate', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '设置 / 在线更新', exact: true }).click();
  await page.getByRole('button', { name: '检查更新', exact: true }).click();
  await page.getByRole('button', { name: '下载更新', exact: true }).click();
  await expect(page.getByRole('button', { name: '安装更新…', exact: true })).toBeVisible();
  await page.getByLabel('更新渠道').selectOption('beta');
  await expect(page.getByRole('button', { name: '安装更新…', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '下载更新', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.desktopTest.installs)).toBe(0);
});

test('directory discovery is explicit, read-only and scoped to the selected provider (simulated host)', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  const detail = page.getByLabel('摸鱼AI 的详情');
  await detail.getByRole('tab', { name: /^模型/ }).click();
  await expect(detail.getByText('demo-model', { exact: true })).toHaveCount(0);
  await detail.getByRole('button', { name: '获取模型目录', exact: true }).dblclick();
  await expect(detail.getByText('demo-model', { exact: true })).toBeVisible();
  await expect(detail).toContainText('已获取 1 个模型 · 42 ms');
  await expect(detail.getByLabel('Key 查询结果')).toContainText('Key anonymous');
  await expect(detail.getByLabel('Key 查询结果')).toContainText('HTTP 200');
  await expect(detail).not.toContainText('目录成功不代表推理可用');
  expect(await page.evaluate(() => ({ saves: window.desktopTest.saves, directories: window.desktopTest.directories }))).toEqual({ saves: 0, directories: 1 });
  await detail.getByRole('button', { name: '返回供应商列表' }).click();
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await detail.getByRole('tab', { name: /^模型/ }).click();
  await expect(detail.getByText('demo-model', { exact: true })).toHaveCount(0);
});

test('an unsupported directory does not label the provider unusable or show upstream errors', async ({ page }) => {
  await host(page);
  await page.evaluate(() => {
    window.go!.desktop!.Bridge!.DiscoverProviderModels = async input => {
      window.desktopTest.directories++;
      return { contract: 'anchor-desktop-v1', data: { provider_id: input.provider_id, key_id: input.key_id || 'anonymous', revision: input.expected_revision,
        http_status: 404, elapsed_ms: 12, models: [], failure: 'unsupported', partial: false, entries: [], keys: [{ id: input.key_id || 'anonymous', enabled: true, protocol: 'responses', http_status: 404, failure: 'unsupported', group: '', group_name: '', multiplier: null, rate_basis: '', rate_failure: 'platform_required' }] }, problem: null };
    };
  });
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  const detail = page.getByLabel('摸鱼AI 的详情');
  await detail.getByRole('tab', { name: /^模型/ }).click();
  await detail.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await expect(detail).toContainText('目录获取失败 · 12 ms');
  await expect(detail.getByLabel('Key 查询结果')).toContainText('Key anonymous');
  await expect(detail.getByLabel('Key 查询结果')).toContainText('HTTP 404');
  await expect(detail).toContainText('不代表模型推理不可用');
  expect(await page.evaluate(() => ({ saves: window.desktopTest.saves, directories: window.desktopTest.directories }))).toEqual({ saves: 0, directories: 1 });
  await expect(detail.getByRole('button', { name: '获取模型目录', exact: true })).toBeEnabled();
});


test('logs reports unavailable without a desktop bridge', async ({ page }) => {
 await page.goto('/');
 await page.getByRole('button', { name: '日志', exact:true }).click();
 await expect(page.getByRole('heading',{name:'日志',exact:true})).toBeVisible();
 await expect(page.getByText('日志读取失败',{exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:/查看请求/})).toHaveCount(0);
});


test('account page configures authorization and keeps wallet and key quota independent', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额' }).click();
  await page.getByLabel('平台类型', { exact: true }).selectOption('newapi');
  await page.getByLabel('NewAPI 用户 ID').fill('7');
  await page.getByLabel('用户令牌 · 未配置', { exact: true }).selectOption('replace');
  await page.getByLabel('平台用户令牌', { exact: true }).fill('synthetic-account-private-token');
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '保存平台配置' }).click();
  await expect(page.getByLabel('用户令牌 · 已保存', { exact: true })).toBeVisible();
  await expect(page.getByLabel('平台用户令牌', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '刷新余额', exact: true }).click();
  const wallet = page.locator('.detail-card').filter({ has: page.getByRole('heading', { name: '账户余额', exact: true }) });
  const quota = page.locator('.detail-card').filter({ has: page.getByRole('heading', { name: 'Key 额度', exact: true }) });
  await expect(wallet.getByText('9007199254740993123 quota（原始额度）', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '刷新额度', exact: true }).click();
  await expect(quota.getByText('9007199254740993123 quota（原始额度）', { exact: true })).toBeVisible();
  await expect(wallet.getByText('9007199254740993123 quota（原始额度）', { exact: true })).toBeVisible();
  await page.evaluate(() => { window.accountTest.failure = 'authentication'; });
  await page.getByRole('button', { name: '刷新余额', exact: true }).click();
  await expect(wallet.getByRole('alert')).toContainText('平台拒绝授权');
  await expect(wallet.getByText('9007199254740993123 quota（原始额度）', { exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain('synthetic-account-private-token');
  expect(await page.evaluate(() => window.accountTest.saves)).toBe(1);
});

test('account unknown save clears the submitted token and requires a read before retry', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额' }).click();
  await page.getByLabel('平台类型', { exact: true }).selectOption('sub2api');
  await page.getByLabel('用户令牌 · 未配置', { exact: true }).selectOption('replace');
  await page.getByLabel('平台用户令牌', { exact: true }).fill('synthetic-unknown-account-token');
  await page.evaluate(() => { window.desktopTest.writeError = 'unknown'; });
  await page.getByRole('button', { name: '保存平台配置' }).click();
  await expect(page.getByRole('alert')).toContainText('写入结果未知');
  await expect(page.getByLabel('平台用户令牌', { exact: true })).toHaveValue('');
  await expect(page.getByRole('button', { name: '保存平台配置' })).toBeDisabled();
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '重新读取配置', exact: true }).click();
  await expect(page.getByLabel('用户令牌 · 已保存', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeEnabled();
  expect(await page.evaluate(() => window.accountTest.saves)).toBe(1);
});

test('catalog pagination, themes and narrow layouts keep Key protocols accessible (simulated host)', async ({ page }) => {
  await host(page);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!;
    const workspace = api.ProviderWorkspace;
    api.ProviderWorkspace = async input => {
      const result = await workspace(input);
      if (result.data) result.data.keys = [
        { id: 'default', enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null, protocol: 'responses', model_protocols: [] },
        { id: 'premium', enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null, protocol: 'anthropic', model_protocols: [] },
      ];
      return result;
    };
    api.DiscoverProviderModels = async input => {
      const names = ['claude-haiku-demo', 'claude-opus-demo', 'claude-sonnet-demo', 'gemini-flash-demo', 'gemini-pro-demo', 'gpt-astra-demo', 'gpt-sol-demo', 'qwen-coder-demo', ...Array.from({ length: 7 }, (_, i) => `long-model-name-for-layout-verification-${i + 9}`)];
      return { contract: 'anchor-desktop-v1', data: {
        provider_id: input.provider_id, key_id: '', revision: input.expected_revision, http_status: 200, elapsed_ms: 376, models: names, failure: '', partial: false,
        entries: names.map((id, i) => ({ id, key_ids: i === 1 ? ['default', 'premium'] : ['default'] })),
        keys: [
          { id: 'default', enabled: true, protocol: 'responses', http_status: 200, failure: '', group: 'default', group_name: '默认组', multiplier: '1', rate_basis: 'authenticated_group', rate_failure: '' },
          { id: 'premium', enabled: true, protocol: 'anthropic', http_status: 200, failure: '', group: 'premium', group_name: '高级组', multiplier: '0.8', rate_basis: 'authenticated_group', rate_failure: '' },
        ],
      }, problem: null };
    };
  });
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  const catalog = page.getByRole('region', { name: '上游模型目录', exact: true });
  await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
  const list = catalog.getByLabel('上游模型 ID');
  await expect(list.locator(':scope > li')).toHaveCount(8);
  await expect(catalog.getByLabel('模型目录上一页')).toBeDisabled();
  await expect(catalog.getByLabel('claude-opus-demo · Key premium 的协议', { exact: true })).toHaveValue('inherit');
  await catalog.getByLabel('模型目录下一页').click();
  await expect(list.locator(':scope > li')).toHaveCount(7);
  await expect(catalog).toContainText('9–15 / 15 个模型');
  await expect(catalog.getByLabel('模型目录下一页')).toBeDisabled();
  await catalog.getByLabel('搜索模型目录').fill('claude');
  await expect(list.locator(':scope > li')).toHaveCount(3);
  await expect(catalog).toContainText('1–3 / 3 个模型');
  await catalog.getByLabel('搜索模型目录').fill('');
  await page.setViewportSize({ width: 1440, height: 1400 });
  for (const theme of ['浅色', '深色']) {
    await page.getByRole('button', { name: theme, exact: true }).click();
    await catalog.scrollIntoViewIfNeeded();
    expect(await list.evaluate(el => el.scrollHeight <= el.clientHeight + 1)).toBe(true);
    if (process.env.ANCHOR_CATALOG_CAPTURE_DIR) await catalog.screenshot({ path: `${process.env.ANCHOR_CATALOG_CAPTURE_DIR}/catalog-${theme === '浅色' ? 'light' : 'dark'}.png` });
  }
  await page.setViewportSize({ width: 760, height: 1800 });
  await catalog.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  const protocol = catalog.getByLabel('claude-opus-demo · Key premium 的协议', { exact: true });
  await expect(protocol).toBeVisible();
  await protocol.focus();
  await expect(protocol).toBeFocused();
  if (process.env.ANCHOR_CATALOG_CAPTURE_DIR) await catalog.screenshot({ path: `${process.env.ANCHOR_CATALOG_CAPTURE_DIR}/catalog-narrow.png` });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await catalog.evaluate(el => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

declare global { interface Window { catalogImportTest: { calls: import('../src/desktop/contract.generated').Action[]; fail: '' | 'unknown' | 'conflict' } } }
for (const failure of ['', 'unknown', 'conflict'] as const) {
  test(`catalog bulk import covers all pages and stops safely ${failure || 'success'} (simulated host)`, async ({ page }) => {
    await host(page);
    await page.evaluate(async failure => {
      const api = window.go!.desktop!.Bridge!;
      const result = await api.ProviderWorkspace({ provider_id: '摸鱼AI' });
      const w = result.data!;
      const mapping = { id: 'already', upstream: 'already-upstream', display_name: 'already', revision: 1, candidate_count: 1, multiplier: 2, strategy: 'priority', tools: false, streaming: false };
      w.models = [mapping];
      window.catalogImportTest = { calls: [], fail: failure };
      api.ProviderWorkspace = async () => ({ contract: 'anchor-desktop-v1', data: structuredClone(w), problem: null });
      const discover = api.DiscoverProviderModels;
      api.DiscoverProviderModels = async input => {
        const result = await discover(input);
        const names = ['already-upstream', 'already', 'vendor/model', ...Array.from({ length: 10 }, (_, i) => `import-${i}`)];
        result.data!.models = names;
        result.data!.entries = names.map(id => ({ id, key_ids: ['anonymous'] }));
        return result;
      };
      api.ProviderAction = async input => {
        window.catalogImportTest.calls.push(input);
        await new Promise(resolve => setTimeout(resolve, 75));
        if (input.expected_revision !== w.revision || w.models.some(m => m.id === input.target)) return { contract: 'anchor-desktop-v1', data: null, problem: { code: 'conflict' } };
        const failed = input.target === 'import-1' ? window.catalogImportTest.fail : '';
        // Unknown may mean the write persisted before the response was lost.
        if (failed === 'conflict') return { contract: 'anchor-desktop-v1', data: null, problem: { code: failed } };
        w.models.push({ ...mapping, id: input.target, upstream: input.model, multiplier: input.multiplier, tools: input.tools, streaming: input.streaming, strategy: input.strategy });
        if (failed === 'unknown') return { contract: 'anchor-desktop-v1', data: null, problem: { code: failed } };
        return { contract: 'anchor-desktop-v1', data: structuredClone(w), problem: null };
      };
    }, failure);
    await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
    await page.getByRole('tab', { name: /^模型/ }).click();
    const catalog = page.getByRole('region', { name: '上游模型目录', exact: true });
    await catalog.getByRole('button', { name: '获取模型目录', exact: true }).click();
    await expect(catalog.getByLabel('上游模型 ID').locator(':scope > li')).toHaveCount(8);
    await expect(catalog.getByRole('button', { name: '快速导入 vendor/model', exact: true })).toBeDisabled();
    await catalog.getByLabel('搜索模型目录').fill('import-0');
    await catalog.getByRole('button', { name: '全部导入（10）', exact: true }).dblclick();
    await expect(catalog.getByRole('status')).toContainText(failure ? '已导入 1 / 10 个' : '已导入 10 / 10 个');
    await expect(catalog.getByRole('status')).toContainText('跳过 1 个已映射模型，2 个需自定义别名');
    const calls = await page.evaluate(() => window.catalogImportTest.calls);
    expect(calls.map(c => c.target)).toEqual(Array.from({ length: failure ? 2 : 10 }, (_, i) => `import-${i}`));
    for (const call of calls) expect(call).toMatchObject({ action: 'model-put', model: call.target, model_revision: 0, multiplier: 1, tools: true, streaming: true });
    if (failure) {
      await expect(catalog.getByRole('button', { name: /^全部导入/ })).toBeDisabled();
      await expect(catalog.getByRole('status')).toContainText('在「import-1」处停止');
      await page.evaluate(() => { window.catalogImportTest.fail = ''; });
      await page.getByRole('button', { name: '重新读取', exact: true }).click();
      const remaining = failure === 'unknown' ? 8 : 9;
      await catalog.getByRole('button', { name: `全部导入（${remaining}）`, exact: true }).click();
      await expect(catalog.getByRole('status')).toContainText(`已导入 ${remaining} / ${remaining} 个`);
      if (failure === 'unknown') expect(await page.evaluate(() => window.catalogImportTest.calls.filter(c => c.target === 'import-1').length)).toBe(1);
      expect(await page.evaluate(() => window.catalogImportTest.calls.filter(c => c.target === 'import-0').length)).toBe(1);
    }
    await page.getByRole('button', { name: '编辑模型 already', exact: true }).click();
    await expect(page.getByRole('dialog').getByLabel('函数工具', { exact: true })).not.toBeChecked();
    await expect(page.getByRole('dialog').getByLabel('此供应商的相对倍率')).toHaveValue('2');
  });
}

test('group rate account reads selected key and removes a failed previous rate (simulated host)', async ({ page }) => {
  await host(page);
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额', exact: true }).click();
  await page.getByRole('button', { name: '刷新分组倍率', exact: true }).click();
  await expect(page.getByText('0.80000000000000001×', { exact: true })).toBeVisible();
  await expect(page.getByText('来源：个人倍率覆盖', { exact: true })).toBeVisible();
  await page.evaluate(() => { window.accountTest.failure = 'authentication'; });
  await page.getByRole('button', { name: '刷新分组倍率', exact: true }).click();
  await expect(page.getByText('0.80000000000000001×', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('alert')).toContainText('平台拒绝授权');
});

test('group rate mappings distinguish routing and platform values and deduplicate providers (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'shared-one', exact: true })).toBeVisible();
  await expect(page.getByText('路由倍率', { exact: false }).filter({ hasText: '2.5×' })).toHaveCount(2);
  const before = await page.evaluate(() => window.accountTest.queries);
  await page.getByRole('button', { name: '刷新分组倍率', exact: true }).dblclick();
  await expect(page.getByText('0.80000000000000001×', { exact: true })).toHaveCount(6);
  expect(await page.evaluate(() => window.accountTest.queries)).toBe(before + 3);
  await page.evaluate(() => { window.accountTest.failure = 'auth_required'; });
  await page.getByRole('button', { name: '查询分组倍率', exact: true }).first().click();
  await expect(page.getByRole('alert').first()).toContainText('查询分组倍率需要平台用户令牌');
  await expect(page.getByText('路由倍率', { exact: false }).filter({ hasText: '2.5×' })).toHaveCount(2);
  await page.getByRole('button', { name: '账户与授权', exact: true }).first().click();
  await expect(page.getByRole('tab', { name: '账户与余额', exact: true })).toHaveAttribute('aria-selected', 'true');
});
