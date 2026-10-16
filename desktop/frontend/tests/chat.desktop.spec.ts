import mappingFixture from '../../contract/provider-model-mapping.json' with { type: 'json' };
import { expect, test, type Page } from '@playwright/test';
import chatFixture from '../../contract/chat.json' with { type: 'json' };
import workspaceFixture from '../../contract/provider-workspace.json' with { type: 'json' };
import catalogFixture from '../../contract/provider-models.json' with { type: 'json' };
import providerFixture from '../../contract/providers.json' with { type: 'json' };
import { CONTRACT, type ChatRequest, type DesktopBridge, type ProbeProgress } from '../src/desktop/contract.generated';

declare global { interface Window { chatTest: { requests: ChatRequest[]; mode: 'success' | 'waiting' | 'failure' | 'unknown'; delayStart: boolean; release: boolean; catalogFails: boolean } } }
async function host(page: Page, modelProtocol = '') {
  await page.addInitScript(({ contract, chat, workspace, catalog, provider, mapping, modelProtocol }) => {
    window.chatTest = { requests: [], mode: 'success', delayStart: false, release: false, catalogFails: false };
    const state = window.chatTest;
    const session = { instance_id: 'chat-test-instance', phase: 'ready', source: 'started', profile_name: '@local', transport: 'local', target: '本机', can_stop: true };
    const rows = [{ ...provider, id: 'chat-demo', revision: 1, enabled: true }, { ...provider, id: 'other', revision: 1, enabled: true }];
    const runs = new Map<string, { input: ChatRequest; reads: number; cancelled: boolean }>();
    const result = (id: string): ProbeProgress => {
      const run = runs.get(id)!; const input = run.input;
      const done = run.cancelled || (state.mode !== 'waiting' && run.reads >= 3);
      return { ...chat.progress, provider_id: input.provider_id, key_id: input.key_id, model: input.model,
        revision: input.expected_revision, run_id: id, elapsed_ms: run.reads * 150,
        text: run.reads ? (done ? '完整回复' : '正在回复') : '', status: run.cancelled ? 'cancelled' : done ? 'completed' : 'running', phase: done ? 'complete' : 'receiving',
        result: done ? { ...chat.progress.result, key_id: input.key_id, model: input.model, revision: input.expected_revision, failure: state.mode === 'failure' ? 'authentication' : '' } : null };
    };
    const api = {
      Start: async () => ({ contract, data: session, problem: null }), Status: async () => ({ contract, data: session, problem: null }),
      Providers: async () => ({ contract, data: rows, problem: null }),
      ProviderWorkspace: async ({ provider_id }: { provider_id: string }) => ({ contract, data: { ...workspace, provider_id, revision: 1,
        keys: [{ ...workspace.keys[0], id: 'default', enabled: true, model_protocols: modelProtocol ? [{ model: 'demo-model', protocol: modelProtocol }] : [] }, { ...workspace.keys[0], id: 'second', enabled: true, protocol: 'chat-completions' }],
        models: [{ ...mapping, key_ids: ['default'], upstream: 'demo-model' }, { ...mapping, key_ids: ['second'], upstream: 'second-model' }],
      }, problem: null }),
      DiscoverProviderModels: async (input: { provider_id: string; key_id: string; expected_revision: number }) => ({ contract, data: state.catalogFails ? null : { ...catalog.success,
        provider_id: input.provider_id, revision: input.expected_revision, key_id: input.key_id,
        models: [input.key_id === 'second' ? 'second-model' : 'demo-model'], entries: [{ id: input.key_id === 'second' ? 'second-model' : 'demo-model', key_ids: [input.key_id] }],
        keys: [{ ...catalog.success.keys[0], id: input.key_id }],
      }, problem: state.catalogFails ? { code: 'read_error' } : null }),
      ProviderChat: async (input: ChatRequest) => {
        state.requests.push(input);
        if (input.action === 'start') {
          if (state.mode === 'unknown') return { contract, data: null, problem: { code: 'unknown' } };
          if (state.delayStart) await new Promise<void>(resolve => { const timer = setInterval(() => { if (state.release) { clearInterval(timer); resolve(); } }, 10); });
          const id = crypto.randomUUID(); runs.set(id, { input, reads: 0, cancelled: false });
          return { contract, data: result(id), problem: null };
        }
        const run = runs.get(input.run_id)!;
        if (input.action === 'cancel') run.cancelled = true; else run.reads++;
        return { contract, data: result(input.run_id), problem: null };
      },
    };
    window.go = { desktop: { Bridge: new Proxy(api, { get: (target, key) => target[key as keyof typeof target] ?? (async () => ({ contract, data: null, problem: { code: 'unsupported' } })) }) as unknown as DesktopBridge } };
  }, { contract: CONTRACT, chat: chatFixture, workspace: workspaceFixture, catalog: catalogFixture, provider: providerFixture.providers[0], mapping: mappingFixture, modelProtocol });
  await page.goto('/'); await page.getByRole('button', { name: '对话测试', exact: true }).click();
  await expect(page.getByLabel('对话模型', { exact: true })).toHaveValue('demo-model');
  await expect(page.getByLabel('对话消息输入')).toBeEnabled();
}
async function send(page: Page, content = '第一条消息') {
  await page.getByLabel('对话消息输入').fill(content); await page.getByRole('button', { name: '发送', exact: true }).click();
}

test('multi-turn streaming, copy, clear and target reset', async ({ page, context }) => {
  await host(page); await send(page);
  await expect(page.getByRole('button', { name: '停止生成', exact: true })).toBeVisible();
  await expect(page.getByLabel('思考深度', { exact: true })).toBeDisabled();
  await expect(page.getByRole('article', { name: '助手回复' })).toContainText('完整回复');
  await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  await page.getByLabel('思考深度', { exact: true }).selectOption('high');
  await expect(page.getByRole('article', { name: '你的消息' })).toHaveCount(1);
  await send(page, '继续刚才的话题');
  await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  const starts = await page.evaluate(() => window.chatTest.requests.filter(r => r.action === 'start'));
  expect(starts).toHaveLength(2);
  expect(starts[0]).not.toHaveProperty('reasoning_effort');
  expect(starts[1].reasoning_effort).toBe('high');
  expect(starts[1].messages).toEqual([{ role: 'user', content: '第一条消息' }, { role: 'assistant', content: '完整回复' }, { role: 'user', content: '继续刚才的话题' }]);
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await page.getByRole('button', { name: '复制第 2 条回复' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('完整回复');
  await page.getByRole('button', { name: '清空对话', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
  await expect(page.getByRole('article', { name: '你的消息' })).toHaveCount(0);
  await send(page, '新会话'); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  await page.getByLabel('对话 Key', { exact: true }).selectOption('second');
  await expect(page.getByLabel('对话模型', { exact: true })).toHaveValue('second-model');
  await expect(page.getByRole('article', { name: '你的消息' })).toHaveCount(0);
  await send(page, '测试第二把 Key'); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  const last = await page.evaluate(() => window.chatTest.requests.filter(r => r.action === 'start').at(-1));
  expect(last?.key_id).toBe('second'); expect(last?.model).toBe('second-model'); expect(last?.messages).toHaveLength(1);
  await page.getByLabel('对话供应商', { exact: true }).selectOption('other');
  await expect(page.getByRole('article', { name: '你的消息' })).toHaveCount(0);
});

test('stop works before the start response and excludes partial context', async ({ page }) => {
  await host(page); await page.evaluate(() => { window.chatTest.mode = 'waiting'; window.chatTest.delayStart = true; });
  await send(page); await page.getByRole('button', { name: '停止生成', exact: true }).click();
  await page.evaluate(() => { window.chatTest.release = true; });
  await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.chatTest.requests.some(r => r.action === 'cancel'))).toBe(true);
  await page.evaluate(() => { window.chatTest.mode = 'success'; window.chatTest.delayStart = false; });
  await send(page, '停止后再发送'); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.chatTest.requests.filter(r => r.action === 'start').at(-1)?.messages)).toHaveLength(1);
});

test('leaving the page cancels even when start is still pending', async ({ page }) => {
  await host(page); await page.evaluate(() => { window.chatTest.mode = 'waiting'; window.chatTest.delayStart = true; });
  await send(page); await page.getByRole('button', { name: '设备连接', exact: true }).click();
  await page.evaluate(() => { window.chatTest.release = true; });
  await expect.poll(() => page.evaluate(() => window.chatTest.requests.some(r => r.action === 'cancel'))).toBe(true);
});

test('manual model, IME-safe Enter, multiline and no automatic retry', async ({ page }) => {
  await host(page); await page.evaluate(() => { window.chatTest.catalogFails = true; });
  await page.getByRole('button', { name: '刷新对话模型目录' }).click();
  await expect(page.getByText(/也可以手动填写模型 ID/)).toBeVisible();
  const model = page.getByLabel('对话模型', { exact: true });
  await model.fill(''); await model.pressSequentially('manual/model');
  await expect(model).toHaveValue('manual/model'); await expect(model).toBeFocused();
  const input = page.getByLabel('对话消息输入'); await input.fill('中文输入');
  await input.dispatchEvent('compositionstart'); await input.press('Enter');
  expect(await page.evaluate(() => window.chatTest.requests)).toHaveLength(0);
  await input.dispatchEvent('compositionend'); await input.press('Shift+Enter'); await input.press('Enter');
  await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.chatTest.requests.find(r => r.action === 'start')?.model)).toBe('manual/model');
  await page.evaluate(() => { window.chatTest.mode = 'unknown'; }); await send(page, '结果未知');
  await expect(page.getByRole('alert')).toContainText('发送结果未知');
  expect(await page.evaluate(() => window.chatTest.requests.filter(r => r.action === 'start'))).toHaveLength(2);
});

test('narrow dark layout stays within viewport', async ({ page }) => {
  await page.setViewportSize({ width: 940, height: 700 }); await page.emulateMedia({ colorScheme: 'dark' });
  await host(page); await send(page, '长文本'.repeat(250)); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  if (process.env.ANCHOR_CHAT_SCREENSHOT) {
    await page.getByRole('button', { name: '清空对话', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确认变更' }).click();
    await send(page, '你好，请简单介绍一下你自己。');
    await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: process.env.ANCHOR_CHAT_SCREENSHOT, fullPage: true });
  }
});


test('reasoning levels follow the Key model protocol override and default can be restored', async ({ page }) => {
  await host(page, 'anthropic');
  const effort = page.getByLabel('思考深度', { exact: true });
  await expect(effort).toHaveValue('');
  expect(await effort.locator('option').evaluateAll(options => options.map(option => (option as HTMLOptionElement).value))).toEqual(['', 'low', 'medium', 'high', 'xhigh', 'max']);
  await effort.selectOption('max');
  await send(page); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  await effort.selectOption('');
  await send(page, '使用模型默认深度'); await expect(page.getByRole('button', { name: '发送', exact: true })).toBeVisible();
  const starts = await page.evaluate(() => window.chatTest.requests.filter(r => r.action === 'start'));
  expect(starts[0].reasoning_effort).toBe('max');
  expect(starts[1]).not.toHaveProperty('reasoning_effort');
  expect(starts[1].messages).toHaveLength(3);
});
