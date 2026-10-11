import { expect, test, type Page } from '@playwright/test';
import fixture from '../../contract/providers.json' with { type: 'json' };
import updateFixture from '../../contract/updates.json' with { type: 'json' };
import { CONTRACT, type DesktopBridge, type Provider } from '../src/desktop/contract.generated';

declare global { interface Window { desktopTest: { directories: number; starts: number; saves: number; stops: number; status: boolean; writeError: 'conflict' | 'unknown' | null; checks: number; downloads: number; installs: number; updateError: boolean } } }
async function host(page: Page) {
  // A fake Wails boundary exercises the real frontend mode, not a native window.
  await page.addInitScript(({ contract, rows, update }) => {
    window.desktopTest = { directories: 0, starts: 0, saves: 0, stops: 0, status: true, writeError: null, checks: 0, downloads: 0, installs: 0, updateError: false };
    const counters = window.desktopTest;
    let providers: Provider[] = rows;
    const data = { instance_id: 'test-local-instance', phase: 'ready', source: 'started', can_stop: true };
    const api: DesktopBridge = {
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
        return { contract, data: { provider_id: provider.id, key_id: input.key_id || 'anonymous', revision: provider.revision, http_status: 200, elapsed_ms: 42, models: ['demo-model'], failure: '' }, problem: null };
      },
      ProviderWorkspace: async input => ({ contract, data: { provider_id: input.provider_id, revision: providers.find(p => p.id === input.provider_id)?.revision ?? 1, strategy: 'round-robin', prices: [], estimates: [], imported_price: null, keys: [], models: [], usage: [], probe: null, deleted: false }, problem: null }),
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
  }, { contract: CONTRACT, rows: fixture.providers, update: updateFixture.downloaded });
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
  await expect(page.getByText('本次启动 · 关闭窗口收起到系统托盘 · 核心持续运行')).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '添加供应商', exact: true }).click();
  await page.getByLabel('供应商名称 / ID').fill('桌面供应商');
  await page.getByLabel('API 根地址', { exact: true }).fill('https://desktop.example.com/v1');
  await page.getByLabel('API 密钥（可选）').fill('desktop-synthetic-secret');
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
  await expect(page.getByText('核心正在处理退出与配置恢复；请求接受不代表恢复已完成。')).toBeVisible();
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
  await expect(page.getByText('复用已有核心 · 关闭窗口收起到系统托盘 · 核心持续运行')).toBeVisible({ timeout: 8000 });
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
  await expect(detail).toContainText('已获取 1 个模型 · Key anonymous · HTTP 200 · 42 ms');
  await expect(detail).toContainText('目录成功不代表推理可用');
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
        http_status: 404, elapsed_ms: 12, models: [], failure: 'unsupported' }, problem: null };
    };
  });
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  const detail = page.getByLabel('摸鱼AI 的详情');
  await detail.getByRole('tab', { name: /^模型/ }).click();
  await detail.getByRole('button', { name: '获取模型目录', exact: true }).click();
  await expect(detail).toContainText('目录获取失败 · Key anonymous · HTTP 404 · 12 ms');
  await expect(detail).toContainText('不代表模型推理不可用');
  expect(await page.evaluate(() => ({ saves: window.desktopTest.saves, directories: window.desktopTest.directories }))).toEqual({ saves: 0, directories: 1 });
  await expect(detail.getByRole('button', { name: '获取模型目录', exact: true })).toBeEnabled();
});


test('logs preview is explicitly synthetic when desktop bridge is unavailable', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible();
  await page.getByRole('button', { name: '日志', exact: true }).click();
  await expect(page.getByRole('heading', { name: '日志', exact: true })).toBeVisible();
  await expect(page.getByText('所有记录均为虚拟数据，用于预览页面与排障交互。')).toBeVisible();
  await page.getByRole('button', { name: '查看请求 req_demo_8f2c91' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '供应商', exact: true }).click();
  await expect(page.getByRole('heading', { name: '本机核心暂不可用' })).toBeVisible();
});
