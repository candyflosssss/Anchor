import modelCatalogFixture from '../../contract/model-catalog.json' with { type: 'json' };
import contextPricingFixture from '../../contract/context-pricing.json' with { type: 'json' };
import modelSettingsFixture from '../../contract/model-settings.json' with { type: 'json' };
import accountFixture from '../../contract/provider-account.json' with { type: 'json' };
import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import updateFixture from '../../contract/updates.json' with { type: 'json' };
import { CONTRACT, type AccountSettings, type DesktopBridge, type KeyRateCache, type ModelSettingsState, type Provider } from '../src/desktop/contract.generated';

declare global { interface Window { desktopTest: { directories: number; starts: number; saves: number; stops: number; status: boolean; writeError: 'conflict' | 'unknown' | 'invalid' | null; checks: number; downloads: number; installs: number; updateError: boolean } } }
declare global { interface Window { accountTest: { saves: number; queries: number; failure: string } } }
async function host(page: Page, rateMappings = false) {
  // A fake Wails boundary exercises the real frontend mode, not a native window.
  await page.addInitScript(({ contract, rows, update, account, modelSettings, rateMappings }) => {
    window.desktopTest = { directories: 0, starts: 0, saves: 0, stops: 0, status: true, writeError: null, checks: 0, downloads: 0, installs: 0, updateError: false };
    const counters = window.desktopTest;
    window.accountTest = { saves: 0, queries: 0, failure: '' };
    const accounts: Record<string, AccountSettings> = {};
    const settings = (id: string): AccountSettings => ({ ...(accounts[id] ?? { ...account.settings, platform: '' }), provider_id: id, revision: providers.find(p => p.id === id)?.revision ?? 1 });
    let providers: Provider[] = rows;
    if (rateMappings) providers = providers.map(p => p.id === '摸鱼AI' ? { ...p, cost_multiplier: 0.5 } : p);
    const rateCache: Record<string, KeyRateCache> = {};
    const data = { instance_id: 'test-local-instance', phase: 'ready', source: 'started', profile_name: '@local', transport: 'local', target: '本机', can_stop: true };
    window.routingTest = { calls: [] };
    const routingState = Object.fromEntries(['shared-one', 'shared-two'].map(id => [id, { revision: 1, strategy: 'round-robin', order: providers.map(p => ({ provider_id: p.id, model: `upstream-${id}` })) }]));
    const modelStates: Record<string, ModelSettingsState> = {};
    const api: DesktopBridge = {
      ProxyWorkspace: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ManageProxy: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
 Connections: async () => ({ contract, data: [], problem: null }),
 ConnectSSH: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
 UseConnection: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ModelCatalog: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      EstimateModelCost: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ProviderChat: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ProviderProbe: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
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
        if (!input.remote_key_id) {
          const cacheID = JSON.stringify([input.provider_id, input.key_id || 'default']);
          const now = new Date().toISOString();
          rateCache[cacheID] = failure
            ? { ...(rateCache[cacheID] ?? { multiplier: null, group: '', group_name: '', basis: '', observed_at: '' }), attempted_at: now, failure }
            : { multiplier: Number(account.rate.rate.multiplier), group: account.rate.rate.group, group_name: account.rate.rate.group_name, basis: account.rate.rate.basis, observed_at: now, attempted_at: now, failure: '' };
        }
        return { contract, data: { ...account.rate, provider_id: input.provider_id, revision: input.expected_revision, platform: settings(input.provider_id).platform || 'sub2api', key_id: input.key_id || 'default', remote_key_id: input.remote_key_id, failure,
          rate: failure ? null : { ...account.rate.rate } }, problem: null };
      },
      ProviderAllowance: async input => {
        window.accountTest.queries++;
        const failure = window.accountTest.failure;
        return { contract, data: { ...account.balance, provider_id: input.provider_id, revision: input.expected_revision, platform: settings(input.provider_id).platform, operation: input.operation, key_id: 'anonymous', remote_key_id: input.remote_key_id, failure,
          allowance: failure ? null : { ...account.balance.allowance, scope: input.operation === 'balance' ? 'account' : 'key', remaining: { value: '9007199254740993123', unit: 'quota' } } }, problem: null };
      },
      SkillWorkspace: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      SkillDetail: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ManageSkill: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      AgentWorkspace: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      EnrollAgent: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ManageAgent: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogSuggestions: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogStats: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      Logs: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogDetail: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      LogSettings: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      ModelSettings: async input => ({ contract, data: modelStates[input.model_id] ?? { ...modelSettings, model_id: input.model_id, revision: routingState[input.model_id]?.revision ?? 1 }, problem: null }),
      SaveModelSettings: async input => {
        const old = modelStates[input.model_id] ?? { ...modelSettings, revision: routingState[input.model_id]?.revision ?? 1 };
        if (old.revision !== input.expected_revision || counters.writeError === 'conflict') return { contract, data: null, problem: { code: 'conflict' } };
        const saved = { ...old, model_id: input.model_id, revision: old.revision + 1, display_name: input.display_name, configuration: input.configuration, channel_retries: input.channel_retries ?? old.channel_retries };
        modelStates[input.model_id] = saved;
        if (routingState[input.model_id]) routingState[input.model_id].revision = saved.revision;
        counters.saves++;
        return { contract, data: saved, problem: null };
      },
      ConfigureModelRouting: async input => {
        window.routingTest.calls.push(input);
        await new Promise(resolve => setTimeout(resolve, 150));
        const m = routingState[input.model_id];
        if (!m || input.expected_revision !== m.revision || counters.writeError === 'conflict') return { contract, data: null, problem: { code: 'conflict' } };
        if (counters.writeError === 'invalid') return { contract, data: null, problem: { code: 'invalid' } };
        m.revision++; m.strategy = input.strategy; m.order = input.order;
        if (counters.writeError === 'unknown') return { contract, data: null, problem: { code: 'unknown' } };
        return { contract, data: { model_id: input.model_id, ...m }, problem: null };
      },
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
      ProviderWorkspace: async input => ({ contract, data: { provider_id: input.provider_id, revision: providers.find(p => p.id === input.provider_id)?.revision ?? 1, strategy: 'round-robin', prices: [], estimates: [], imported_price: null, keys: rateMappings ? [{ rate: rateCache[JSON.stringify([input.provider_id, 'default'])] ?? null, effective_multiplier: rateCache[JSON.stringify([input.provider_id, 'default'])]?.multiplier != null ? providers.find(p => p.id === input.provider_id)!.cost_multiplier * rateCache[JSON.stringify([input.provider_id, 'default'])].multiplier! : null, id: 'default', protocol: 'responses', model_protocols: [], enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null }] : [], models: rateMappings ? ['shared-one', 'shared-two'].map(id => ({ key_ids: ['default'], candidate_index: routingState[id].order.findIndex(p => p.provider_id === input.provider_id), id, upstream: `upstream-${id}`, display_name: id, revision: routingState[id].revision, candidate_count: 3, multiplier: input.provider_id === '摸鱼AI' ? 2.5 : 1.2, strategy: routingState[id].strategy, tools: true, streaming: true })) : [], usage: [], probe: null, deleted: false }, problem: null }),
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
      RestartCore: async () => ({ contract, data: null, problem: { code: 'unsupported' } }),
      StopCore: async input => {
        if (!input.approved || input.instance_id !== data.instance_id) return { contract, data: null, problem: { code: 'invalid' } };
        counters.stops++; return { contract, data: { accepted: true }, problem: null };
      },
    };
    window.go = { desktop: { Bridge: api } };
  }, { contract: CONTRACT, rows: fixture.providers, update: updateFixture.downloaded, account: accountFixture, modelSettings: modelSettingsFixture, rateMappings });
  await page.goto('/');
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
}
test('desktop browser without Wails bindings fails closed instead of showing mock data', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible();
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
  await expect(page.getByRole('dialog', { name: '切换设备' })).toBeVisible();
  await expect(page.getByRole('dialog', { name: '切换设备' })).not.toContainText('test-local-instance');
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
  await expect(page.getByRole('heading', { name: '当前设备暂不可用' })).toBeVisible({ timeout: 8000 });
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
      return { ...result, data: result.data ? { ...result.data, source: 'reused', profile_name: '@local', transport: 'local', target: '本机', can_stop: false } : null };
    };
  });
  await page.getByRole('button', { name: /本机实例 · 已连接/ }).click();
  await expect(page.getByRole('button', { name: '停止核心', exact: true })).toHaveCount(0, { timeout: 8000 });
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
  await page.getByLabel('NewAPI 系统访问令牌 · 未配置', { exact: true }).selectOption('replace');
  await page.getByLabel('NewAPI 系统访问令牌', { exact: true }).fill('synthetic-account-private-token');
  await expect(page.getByRole('button', { name: '刷新余额', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '保存平台配置' }).click();
  await expect(page.getByLabel('NewAPI 系统访问令牌 · 已保存', { exact: true })).toBeVisible();
  await expect(page.getByLabel('NewAPI 系统访问令牌', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '刷新余额', exact: true }).click();
  const wallet = page.locator('.detail-card').filter({ has: page.getByRole('heading', { name: '余额', exact: true }) });
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
        { rate: null, effective_multiplier: null, id: 'default', enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null, protocol: 'responses', model_protocols: [] },
        { rate: null, effective_multiplier: null, id: 'premium', enabled: true, state: 'ready', failures: 0, retry_at: 0, test: null, protocol: 'anthropic', model_protocols: [] },
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
      const mapping = { key_ids: [] as string[], candidate_index: 0, id: 'already', upstream: 'already-upstream', display_name: 'already', revision: 1, candidate_count: 1, multiplier: 2, strategy: 'priority', tools: false, streaming: false };
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
        w.models.push({ ...mapping, id: input.target, upstream: input.model, key_ids: input.key_ids ?? [], multiplier: input.multiplier, tools: input.tools, streaming: input.streaming, strategy: input.strategy });
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
    await expect(page.getByRole('dialog').getByLabel('此供应商的相对倍率')).toHaveCount(0);
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
  if (process.env.ANCHOR_MAPPING_CAPTURE_DIR) await page.addInitScript(() => {
    Object.assign(window, { runtime: { WindowMinimise() {}, WindowToggleMaximise() {}, WindowIsMaximised: async () => false, Quit() {} } });
  });
  await host(page, true);
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'shared-one', exact: true })).toBeVisible();
  for (const summary of await page.locator('.model-sources > summary').all()) await summary.click();
  for (const summary of await page.locator('.mapping-keys > summary').all()) await summary.click();
  await expect(page.getByText('供应商倍率 0.5×', { exact: true })).toHaveCount(2);
  const before = await page.evaluate(() => window.accountTest.queries);
  await page.getByRole('button', { name: '刷新分组倍率', exact: true }).dblclick();
  await expect(page.locator('.key-multiplier').filter({ hasText: '分组倍率 0.8×' })).toHaveCount(6);
  expect(await page.evaluate(() => window.accountTest.queries)).toBe(before + 3);
  await expect(page.locator('.key-multiplier').filter({ hasText: '相对倍率 0.4×' })).toHaveCount(2);
  await page.evaluate(() => { window.accountTest.failure = 'auth_required'; });
  await page.getByRole('button', { name: '刷新分组倍率', exact: true }).click();
  await expect(page.getByRole('alert').first()).toContainText('用户令牌');
  await expect(page.locator('.key-multiplier').filter({ hasText: '刷新失败，沿用上次成功缓存。' })).toHaveCount(6);
  await expect(page.getByText('供应商倍率 0.5×', { exact: true })).toHaveCount(2);
  await page.locator('.mapping-candidate .candidate').first().click();
  await page.getByRole('tab', { name: '账户与余额', exact: true }).click();
  await expect(page.getByRole('tab', { name: '账户与余额', exact: true })).toHaveAttribute('aria-selected', 'true');
  if (process.env.ANCHOR_MAPPING_CAPTURE_DIR) {
    // Representative screenshot data stays behind the simulated Wails boundary.
    await page.evaluate(() => {
      window.accountTest.failure = '';
      const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
      api.ProviderWorkspace = async input => {
        const result = await read(input);
        if (result.data) {
          const aliases = input.provider_id === '摸鱼AI' ? ['claude-opus-4-6', 'codex-auto-review', 'gpt-6-astra', 'gpt-6.1-sol'] : input.provider_id === '备用供应商' ? ['claude-opus-4-6', 'claude-opus-5-5', 'claude-sonnet-5-5'] : [];
          result.data.models = aliases.map(id => ({ ...result.data!.models[0], id, upstream: id, display_name: id, candidate_count: id === 'claude-opus-4-6' ? 2 : 1, candidate_index: id === 'claude-opus-4-6' && input.provider_id === '摸鱼AI' ? 1 : 0, key_ids: ['default'], multiplier: 1, strategy: 'priority' }));
        }
        return result;
      };
    });
    await page.getByRole('button', { name: '模型映射', exact: true }).click();
    await page.getByRole('button', { name: '刷新配置', exact: true }).click();
    await expect(page.getByRole('article', { name: '模型 claude-opus-4-6', exact: true })).toBeVisible();
    await page.setViewportSize({ width: 1789, height: 1100 });
    await page.getByRole('button', { name: '浅色', exact: true }).click();
    await page.getByRole('article', { name: '模型 claude-opus-4-6', exact: true }).locator('.model-sources > summary').click();
    await page.getByRole('button', { name: '拖动 备用供应商 claude-opus-4-6', exact: true }).hover();
    await page.screenshot({ path: `${process.env.ANCHOR_MAPPING_CAPTURE_DIR}/model-mappings-light.png` });
    await page.setViewportSize({ width: 820, height: 1000 });
    await page.screenshot({ path: `${process.env.ANCHOR_MAPPING_CAPTURE_DIR}/model-mappings-narrow.png`, fullPage: true });
  }
});


declare global { interface Window { routingTest: { calls: import('../src/desktop/contract.generated').ModelRouting[] } } }

test('model cards collapse providers and Keys with a consistent default layout (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
    api.ProviderWorkspace = async input => {
      const result = await read(input);
      if (result.data) result.data.models = result.data.models.flatMap(m => m.id === 'shared-two' ? input.provider_id === '摸鱼AI' ? [{ ...m, candidate_count: 1, candidate_index: 0 }] : [] : [m]);
      return result;
    };
  });
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  const single = page.getByRole('article', { name: '模型 shared-two', exact: true });
  await expect(single).toContainText('1 个来源 · 1 个 Key');
  await expect(single.locator('.route-grip')).toHaveCount(0);

  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await expect(card).toContainText('3 个来源 · 3 个 Key');
  await expect(card.locator('.model-sources')).not.toHaveAttribute('open', '');
  const shape = (node: Element) => { const style = getComputedStyle(node); return [style.padding, style.backgroundColor, style.borderRadius, node.getBoundingClientRect().height]; };
  expect(await single.evaluate(shape)).toEqual(await card.evaluate(shape));
  if (process.env.ANCHOR_MAPPING_CAPTURE_DIR) {
    await page.screenshot({ path: `${process.env.ANCHOR_MAPPING_CAPTURE_DIR}/models-default.png` });
    await page.getByRole('button', { name: '深色', exact: true }).click();
    await page.screenshot({ path: `${process.env.ANCHOR_MAPPING_CAPTURE_DIR}/models-default-dark.png` });
    await page.getByRole('button', { name: '浅色', exact: true }).click();
  }

  await expect(card.locator('li.mapping-candidate').first()).not.toBeVisible();
  await card.locator('.model-sources > summary').click();
  await expect(card.locator('li.mapping-candidate').first()).toBeVisible();
  await expect(card.locator('.mapping-keys[open]')).toHaveCount(0);
  await card.locator('.mapping-keys > summary').first().click();
  await expect(card.getByLabel('来源 Key default', { exact: true }).first()).toBeVisible();
  await card.locator('.model-sources > summary').click();
  await expect(card.locator('li.mapping-candidate').first()).not.toBeVisible();
  await expect(card.getByRole('button', { name: '模型设置 shared-one', exact: true })).toBeVisible();
});

test('model routing previews pointer sorting and automatically saves rules with undo (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.locator('.model-sources > summary').click();
  const sources = card.locator('li.mapping-candidate');
  const handle = await card.getByRole('button', { name: '拖动 摸鱼AI upstream-shared-one', exact: true }).boundingBox();
  const target = await sources.nth(1).boundingBox();
  expect(handle).toBeTruthy(); expect(target).toBeTruthy();
  await page.mouse.move(handle!.x + handle!.width / 2, handle!.y + handle!.height / 2);
  await page.mouse.down(); await page.mouse.move(target!.x + 40, target!.y + target!.height / 2, { steps: 8 });
  await expect(card.locator('.route-drag-preview')).toBeVisible();
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(0);
  await page.mouse.up();
  await expect(card.locator('.route-save-state')).toContainText('已自动保存');
  await expect(sources.nth(1)).toContainText('摸鱼AI');
  await card.getByLabel('调度规则').selectOption('lowest-price');
  await expect(card).toContainText('倍率不等于实际单价');
  await expect(card.locator('.route-save-state')).toContainText('已自动保存');
  const calls = await page.evaluate(() => window.routingTest.calls);
  expect(calls).toHaveLength(2); expect(calls[1].expected_revision).toBe(2); expect(calls[1].strategy).toBe('lowest-price'); expect(calls[1].order[1].provider_id).toBe('摸鱼AI');
  await card.getByRole('button', { name: '撤销', exact: true }).click();
  await expect(card.locator('.route-save-state')).toContainText('已撤销并保存');
  await expect(card.getByLabel('调度规则')).toHaveValue('round-robin');
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(3);
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await card.locator('.model-sources > summary').click();
  await expect(card.locator('li.mapping-candidate').nth(1)).toContainText('摸鱼AI');
});

test('model routing cancels pointer sorting and supports keyboard ordering (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.locator('.model-sources > summary').click();
  const grip = card.getByRole('button', { name: '拖动 摸鱼AI upstream-shared-one', exact: true });
  const box = await grip.boundingBox();
  await page.mouse.move(box!.x + 10, box!.y + 10); await page.mouse.down(); await page.mouse.move(box!.x + 10, box!.y + 130, { steps: 6 });
  await expect(card.locator('.route-drag-preview')).toBeVisible();
  await page.keyboard.press('Escape'); await page.mouse.up();
  await expect(card.locator('.route-drag-preview')).toHaveCount(0);
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(0);
  await grip.focus(); await page.keyboard.press('ArrowDown');
  await expect(card.locator('.route-save-state')).toContainText('已自动保存');
  await expect(card.locator('li.mapping-candidate').nth(1)).toContainText('摸鱼AI');
  await expect(grip).toBeFocused();
});

for (const failure of ['conflict', 'unknown'] as const) test(`model routing ${failure} requires a read without replaying (simulated host)`, async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.locator('.model-sources > summary').click();
  await page.evaluate(value => { window.desktopTest.writeError = value; }, failure);
  await card.getByRole('button', { name: '下移 摸鱼AI upstream-shared-one', exact: true }).click();
  await expect(card.getByRole('alert')).toContainText(failure === 'conflict' ? '未覆盖新版本' : '结果未知');
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(1);
  await expect(card.getByLabel('调度规则')).toBeDisabled();
  await page.evaluate(() => { window.desktopTest.writeError = null; });
  await card.getByRole('button', { name: '重新读取配置', exact: true }).click();
  await expect(card.getByLabel('调度规则')).toBeEnabled();
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(1);
  await expect(card.locator('li.mapping-candidate').nth(failure === 'unknown' ? 1 : 0)).toContainText('摸鱼AI');
});

test('model settings opens from each card, saves context and reasoning and survives reopening (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toBeVisible();
  await dialog.getByLabel('上下文窗口（tokens）').fill('300000');
  await dialog.locator('summary').filter({ hasText: /^思考设置$/ }).click();
  await dialog.getByLabel('默认思考等级').selectOption('"low"');
  expect(await dialog.locator('input:invalid').count()).toBe(0);
  await dialog.getByRole('button', { name: '保存模型设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  await dialog.getByRole('button', { name: '关闭编辑面板', exact: true }).click();
  await card.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('300000');
  await dialog.locator('summary').filter({ hasText: /^思考设置$/ }).click();
  await expect(dialog.getByLabel('默认思考等级')).toHaveValue('"low"');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
});


test('model routing keeps a failed draft for explicit retry or discard (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.locator('.model-sources > summary').click();
  await page.evaluate(() => { window.desktopTest.writeError = 'invalid'; });
  await card.getByLabel('调度规则').selectOption('priority');
  await expect(card.getByRole('button', { name: '重试保存', exact: true })).toBeVisible();
  await expect(card.getByLabel('调度规则')).toBeDisabled();
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(1);
  await card.getByRole('button', { name: '放弃修改', exact: true }).click();
  await expect(card.getByLabel('调度规则')).toHaveValue('round-robin');
  await card.getByLabel('调度规则').selectOption('priority');
  await expect(card.getByRole('button', { name: '重试保存', exact: true })).toBeVisible();
  await page.evaluate(() => { window.desktopTest.writeError = null; });
  await card.getByRole('button', { name: '重试保存', exact: true }).dblclick();
  await expect(card.locator('.route-save-state')).toContainText('已自动保存');
  await expect(card.getByLabel('调度规则')).toHaveValue('priority');
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(3);
});

test('model routing does not retry an acknowledged write when reloading fails (simulated host)', async ({ page }) => {
  await host(page, true); await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.locator('.model-sources > summary').click();
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.Providers;
    api.Providers = async () => ({ contract: 'anchor-desktop-v1', data: [], problem: { code: 'read_error' } });
    Object.assign(window, { restoreModelRead: () => { api.Providers = read; } });
  });
  await card.getByLabel('调度规则').selectOption('priority');
  await expect(page.getByRole('alert').filter({ hasText: '修改已保存' })).toBeVisible();
  await expect(page.getByRole('button', { name: '重试保存', exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(1);
  await page.evaluate(() => { (window as unknown as { restoreModelRead: () => void }).restoreModelRead(); });
  await page.getByRole('button', { name: '重新读取配置', exact: true }).click();
  await expect(card).toBeVisible();
  await expect(card.locator('.route-save-state')).toContainText('已自动保存');
  await expect(card.locator('.model-strategy')).toContainText('固定顺序');
  expect(await page.evaluate(() => window.routingTest.calls.length)).toBe(1);
});

test('mapping availability follows scoped Keys and provider switches, not provider-wide health (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
    api.ProviderWorkspace = async input => {
      const result = await read(input);
      if (result.data) {
        result.data.keys = [
          { ...result.data.keys[0], id: 'scoped', enabled: false, state: 'disabled' },
          { ...result.data.keys[0], id: 'unrelated', enabled: true, state: 'ready' },
        ];
        result.data.models = result.data.models.map(m => ({ ...m, key_ids: ['scoped'] }));
      }
      return result;
    };
  });
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await expect(card).toContainText('0 / 3 个渠道当前可用');
  await expect(card.locator('.model-unavailable-notice')).toContainText('无有效渠道');
  await card.locator('.model-sources > summary').click();
  await expect(card.locator('.mapping-channel-state').filter({ hasText: '供应商已停用' })).toHaveCount(1);
  await card.locator('.mapping-keys > summary').first().click();
  await expect(card.locator('.mapping-key-status').first()).toContainText('Key 已停用');
  await expect(card.locator('.mapping-key-rate').filter({ hasText: 'unrelated' })).toHaveCount(0);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
    api.ProviderWorkspace = async input => {
      const result = await read(input);
      if (result.data) result.data.keys = result.data.keys.map(k => k.id === 'scoped' ? { ...k, enabled: true, state: 'ready' } : k);
      return result;
    };
  });
  await page.getByRole('button', { name: '刷新配置', exact: true }).click();
  await expect(card).toContainText('2 / 3 个渠道当前可用');
  await expect(card.locator('.model-unavailable-notice')).toHaveCount(0);
  await expect(card.locator('.mapping-key-status').first()).toContainText('Key 已启用');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(0);
});

declare global { interface Window { speedSettingsTest: { calls: import('../src/desktop/contract.generated').Action[]; failure: 'conflict' | 'unknown' | null } } }
for (const failure of ['conflict', 'unknown'] as const) test(`mapping speed settings blocks ${failure} retries until explicit reread (simulated host)`, async ({ page }) => {
  await host(page, true);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
    let tier = '', revision = 1;
    window.speedSettingsTest = { calls: [], failure: null };
    api.ProviderWorkspace = async input => {
      const result = await read(input);
      if (result.data) result.data.models = result.data.models.map(m => m.id === 'shared-one' ? { ...m, service_tier: tier, revision } : m);
      return result;
    };
    api.ProviderAction = async input => {
      window.speedSettingsTest.calls.push(input);
      await new Promise(resolve => setTimeout(resolve, 150));
      const failure = window.speedSettingsTest.failure;
      if (failure !== 'conflict') { tier = input.service_tier ?? tier; revision++; }
      if (failure) return { contract: 'anchor-desktop-v1', data: null, problem: { code: failure } };
      return api.ProviderWorkspace({ provider_id: input.provider_id });
    };
  });
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const card = page.getByRole('article', { name: '模型 shared-one', exact: true });
  await card.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: '速度与分身', exact: true }).click();
  await dialog.getByLabel('分身来源渠道').selectOption('摸鱼AI');
  const tier = dialog.getByLabel('Anchor 固定速度档位');
  await expect(tier).toBeEnabled();
  await tier.selectOption('priority');
  await page.evaluate(value => { window.speedSettingsTest.failure = value; }, failure);
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).dblclick();
  await expect(dialog.getByRole('alert')).toContainText(failure === 'unknown' ? '写入结果未知' : '配置已被其他操作修改');
  await expect(tier).toBeDisabled();
  await expect(dialog.getByRole('button', { name: '保存速度设置', exact: true })).toBeDisabled();
  expect(await page.evaluate(() => window.speedSettingsTest.calls)).toMatchObject([{ target: 'shared-one', model: 'upstream-shared-one', model_revision: 1, expected_revision: 3, key_ids: ['default'], strategy: 'round-robin', tools: true, streaming: true, service_tier: 'priority', speed_variants: [] }]);
  await page.evaluate(() => { window.speedSettingsTest.failure = null; });
  await dialog.getByRole('button', { name: '重新读取速度设置', exact: true }).click();
  await expect(tier).toHaveValue(failure === 'unknown' ? 'priority' : '');
  await expect(tier).toBeEnabled();
  await tier.selectOption('ultrafast');
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('速度设置已保存');
  expect(await page.evaluate(() => window.speedSettingsTest.calls.length)).toBe(2);
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(card).toContainText('Anchor 固定档位：Ultrafast');
});

test('mapping speed settings cannot replay an acknowledged malformed result or a failed reread (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.evaluate(() => {
    const api = window.go!.desktop!.Bridge!, read = api.ProviderWorkspace;
    window.speedSettingsTest = { calls: [], failure: null };
    api.ProviderAction = async input => {
      window.speedSettingsTest.calls.push(input);
      const result = await read({ provider_id: input.provider_id });
      if (result.data) result.data.provider_id = 'different-provider';
      return result;
    };
    api.ProviderWorkspace = async input => window.desktopTest.writeError
      ? { contract: 'anchor-desktop-v1', data: null, problem: { code: 'read_error' } } : read(input);
  });
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: '速度与分身', exact: true }).click();
  await dialog.getByLabel('分身来源渠道').selectOption('摸鱼AI');
  const tier = dialog.getByLabel('Anchor 固定速度档位');
  await expect(tier).toBeEnabled();
  await tier.selectOption('priority');
  await dialog.getByRole('button', { name: '保存速度设置', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('速度设置已保存，但返回配置无法核对');
  await expect(dialog.getByRole('button', { name: '保存速度设置', exact: true })).toBeDisabled();
  await page.evaluate(() => { window.desktopTest.writeError = 'invalid'; });
  await dialog.getByRole('button', { name: '重新读取速度设置', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('未能读取供应商配置');
  await expect(dialog.getByRole('button', { name: '保存速度设置', exact: true })).toHaveCount(0);
  await page.evaluate(() => { window.desktopTest.writeError = null; });
  await dialog.getByRole('button', { name: '重新读取速度设置', exact: true }).click();
  await expect(tier).toBeEnabled();
  expect(await page.evaluate(() => window.speedSettingsTest.calls.length)).toBe(1);
});

test('model settings automatically fills defaults and saves the matched reference only on confirmation (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.evaluate(({ configuration, catalog }) => {
    const api = window.go!.desktop!.Bridge!;
    const state: ModelSettingsState = { ...configuration, model_id: 'shared-one', revision: 1, defaults_status: 'matched', reference: catalog, context_policy: catalog.models[0].context_policy };
    state.configuration.limits = { context: 250000, input: 250000, output: 100000 };
    api.ModelSettings = async () => ({ contract: 'anchor-desktop-v1', data: structuredClone(state), problem: null });
    api.SaveModelSettings = async input => {
      if (input.reference_action !== 'set' || input.reference_provider_id !== 'lab' || input.reference_model_id !== 'vision-reasoner') throw Error('missing automatic reference');
      window.desktopTest.saves++;
      state.configuration = input.configuration; state.revision++; state.defaults_status = 'saved';
      return { contract: 'anchor-desktop-v1', data: structuredClone(state), problem: null };
    };
  }, { configuration: modelSettingsFixture, catalog: modelCatalogFixture.catalog });
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('250000');
  await expect(dialog.getByLabel('最大输出（tokens）', { exact: true })).toHaveValue('100000');
  await expect(dialog.getByRole('status')).toHaveCount(0);
  await expect(dialog.locator('summary').filter({ hasText: '目录与参考价格' })).toContainText('已补全');
  await dialog.locator('summary').filter({ hasText: '目录与参考价格' }).click();
  await expect(dialog.getByText('已自动从 models.dev', { exact: false })).toBeVisible();
  await expect(dialog.getByLabel('目录供应商')).toHaveValue('lab');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(0);
  await expect(dialog.getByText('模型上限 400,000', { exact: false })).toBeVisible();
  await dialog.getByLabel('上下文窗口（tokens）').fill('500000');
  await expect(dialog.getByRole('button', { name: '保存模型设置', exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '重新读取并自动补全', exact: true }).click();
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('250000');
  await dialog.getByRole('button', { name: '保存模型设置', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
});


test('model settings exposes long context prices and requires acknowledgement without losing saved values (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.evaluate(({ configuration, catalog, priced }) => {
    const api = window.go!.desktop!.Bridge!;
    const state: ModelSettingsState = { ...configuration, model_id: 'shared-one', revision: 1, defaults_status: 'reference', context_policy: priced.context_policy, reference: { ...catalog, models: [priced] } };
    state.configuration.limits = { context: 500000, input: 400000, output: 128000 };
    api.ModelSettings = async () => ({ contract: 'anchor-desktop-v1', data: structuredClone(state), problem: null });
    api.SaveModelSettings = async input => {
      if (input.configuration.limits.context === 272000 && input.configuration.limits.input !== 272000) throw Error('hidden input not reconciled');
      if (input.reference_action !== 'keep') throw Error('changed pinned reference');
      window.desktopTest.saves++;
      state.configuration = input.configuration; state.revision++;
      return { contract: 'anchor-desktop-v1', data: structuredClone(state), problem: null };
    };
  }, { configuration: modelSettingsFixture, catalog: modelCatalogFixture.catalog, priced: contextPricingFixture });
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const save = dialog.getByRole('button', { name: '保存模型设置', exact: true });
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('500000');
  await expect(dialog.getByText('模型上限 1,050,000', { exact: false })).toBeVisible();
  await dialog.locator('summary').filter({ hasText: '目录与参考价格' }).click();
  await expect(dialog.getByRole('table')).toContainText('≤ 272,000');
  await expect(dialog.getByRole('table')).toContainText('> 272,000');
  await expect(save).toBeDisabled();
  await dialog.getByLabel('我已了解跨档费用，保留此窗口').check();
  await expect(save).toBeEnabled();
  await dialog.getByLabel('上下文窗口（tokens）').fill('500001');
  await expect(save).toBeDisabled();
  await dialog.getByRole('button', { name: '使用基础价参考窗口（272,000）', exact: true }).click();
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('272000');
  await expect(save).toBeEnabled();
  await save.click();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  await dialog.getByLabel('上下文窗口（tokens）').fill('500000');
  await dialog.getByLabel('我已了解跨档费用，保留此窗口').check();
  await save.click();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  await dialog.getByRole('button', { name: '重新读取并自动补全', exact: true }).click();
  await expect(dialog.getByLabel('上下文窗口（tokens）')).toHaveValue('500000');
  await expect(save).toBeDisabled();
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(2);
});


test('mapping retry count defaults, validates, persists zero and locks on conflict (simulated host)', async ({ page }) => {
  await host(page, true);
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  const open = async () => {
    await page.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  };
  await open();
  const dialog = page.getByRole('dialog'), retries = dialog.getByLabel('同一渠道重试次数');
  const save = dialog.getByRole('button', { name: '保存模型设置', exact: true });
  await expect(retries).toHaveValue('3');
  for (const invalid of ['11', '-1', '1.5', '']) {
    await retries.fill(invalid);
    await expect(save).toBeDisabled();
    await expect(dialog.getByRole('alert')).toContainText('0–10 的整数');
  }
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(0);
  await retries.fill('0');
  await save.dblclick();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
  await dialog.getByRole('button', { name: '关闭编辑面板', exact: true }).click();
  await open(); await expect(retries).toHaveValue('0');
  await retries.fill('2');
  await page.evaluate(() => { window.desktopTest.writeError = 'conflict'; });
  await save.click();
  await expect(dialog.getByRole('alert')).toContainText('配置已被其他操作修改');
  await expect(retries).toBeDisabled(); await expect(save).toBeDisabled();
  await page.evaluate(() => { window.desktopTest.writeError = null; });
  await dialog.getByRole('button', { name: '重新读取设置', exact: true }).click();
  await expect(retries).toHaveValue('0'); await expect(retries).toBeEnabled();
});


test('model settings opens on a compact general form with a fixed footer in light, dark and small windows (simulated host)', async ({ page }, testInfo) => {
  await host(page, true);
  await page.getByRole('button', { name: '模型映射', exact: true }).click();
  await page.getByRole('button', { name: '模型设置 shared-one', exact: true }).click();
  const dialog = page.getByRole('dialog'), save = dialog.getByRole('button', { name: '保存模型设置', exact: true });
  await expect(dialog.getByRole('button', { name: '常规设置', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(dialog.getByLabel('同一渠道重试次数')).toBeVisible();
  await expect(dialog.getByLabel('同一渠道重试次数')).toHaveValue('3');
  for (const title of ['模型能力', '思考设置', '高级设置', '目录与参考价格']) {
    await expect(dialog.locator('details').filter({ has: page.locator('summary').filter({ hasText: title }) })).not.toHaveAttribute('open');
  }
  await expect(dialog.getByLabel('视觉输入')).toBeHidden();
  await expect(dialog.getByLabel('目录供应商')).toBeHidden();
  for (const viewport of [{ width: 1120, height: 860 }, { width: 800, height: 600 }, { width: 480, height: 680 }]) {
    await page.setViewportSize(viewport);
    for (const theme of ['light', 'dark']) {
      await page.evaluate(dark => document.documentElement.classList.toggle('dark', dark), theme === 'dark');
      await expect(save).toBeInViewport();
      await expect(dialog.getByLabel('同一渠道重试次数')).toBeInViewport();
      expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
      const bounds = await dialog.boundingBox();
      expect(bounds!.y).toBeGreaterThanOrEqual(0);
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
      await dialog.screenshot({ path: testInfo.outputPath(`model-settings-${viewport.width}-${theme}.png`) });
    }
  }
  const capabilities = dialog.locator('summary').filter({ hasText: /^模型能力$/ });
  await capabilities.click();
  await capabilities.press('Enter');
  await expect(dialog.getByLabel('视觉输入')).toBeHidden();
  await capabilities.press('Enter');
  await expect(dialog.getByLabel('视觉输入')).toBeVisible();
  await dialog.getByLabel('视觉输入').selectOption('no');
  await expect(save).toBeInViewport();
  await dialog.locator('.model-settings-body').evaluate(el => { el.scrollTop = el.scrollHeight; });
  await expect(save).toBeInViewport();
  await save.click();
  await expect(dialog.getByRole('status')).toContainText('模型设置已保存');
  expect(await page.evaluate(() => window.desktopTest.saves)).toBe(1);
  await dialog.getByRole('button', { name: '速度与分身', exact: true }).click();
  await expect(dialog.getByRole('button', { name: '速度与分身', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(dialog.getByLabel('分身来源渠道')).toBeVisible();
  await dialog.getByRole('button', { name: '常规设置', exact: true }).click();
  await expect(dialog.getByLabel('同一渠道重试次数')).toHaveValue('3');
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
