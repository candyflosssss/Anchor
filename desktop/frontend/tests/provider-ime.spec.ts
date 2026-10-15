import { expect, test, type Locator, type Page } from '@playwright/test';

async function openProvider(page: Page) {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
}

async function composingKey(input: Locator, key: string, options: { isComposing?: boolean; keyCode?: number } = {}) {
  return input.evaluate((node, event) => !node.dispatchEvent(new KeyboardEvent('keydown', {
    ...event, bubbles: true, cancelable: true,
  })), { key, ...options });
}

test('Key name keeps IME preedit, candidate Enter/Escape do not submit or close, committed Chinese saves', async ({ page }) => {
  await openProvider(page);
  await page.getByRole('tab', { name: /^Key/ }).click();
  await page.getByRole('button', { name: '添加 Key', exact: true }).click();
  const input = page.getByLabel('Key 名称', { exact: true });
  await input.fill('');
  await page.getByLabel('API Key', { exact: true }).fill('synthetic-ime-test-only');
  await input.focus();
  const session = await page.context().newCDPSession(page);
  // Real Chromium composition events, not fill(Chinese) or paste.
  await session.send('Input.imeSetComposition', { text: 'zhong ', selectionStart: 6, selectionEnd: 6 });
  await expect(input).toHaveValue('zhong ');
  await session.send('Input.imeSetComposition', { text: '中文测试', selectionStart: 4, selectionEnd: 4 });
  await expect(input).toHaveValue('中文测试');
  expect(await composingKey(input, 'Enter', { isComposing: true })).toBe(true);
  expect(await composingKey(input, 'Escape', { isComposing: true })).toBe(true);
  await expect(page.getByRole('dialog')).toBeVisible();
  await session.send('Input.insertText', { text: '中文测试' });
  expect(await composingKey(input, 'Enter', { keyCode: 229 })).toBe(true);
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(input).toHaveValue('中文测试');
  // Ordinary Enter after composition must still save normally.
  await input.press('Enter');
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByRole('button', { name: '编辑 Key 中文测试', exact: true })).toBeVisible();
  await session.detach();
});

test('IME key handling also honors composition state when WebView isComposing is false', async ({ page }) => {
  await openProvider(page);
  await page.getByRole('tab', { name: /^Key/ }).click();
  await page.getByRole('button', { name: '添加 Key', exact: true }).click();
  const input = page.getByLabel('Key 名称', { exact: true });
  await input.fill('中文候选');
  await page.getByLabel('API Key', { exact: true }).fill('synthetic-ime-test-only');
  await input.dispatchEvent('compositionstart');
  expect(await composingKey(input, 'Enter')).toBe(true);
  expect(await composingKey(input, 'Escape')).toBe(true);
  await expect(page.getByRole('dialog')).toBeVisible();
  await input.dispatchEvent('compositionend', { data: '中文候选' });
  await input.press('Escape');
  await expect(page.getByRole('dialog')).toBeHidden();
});

for (const kind of ['rename', 'copy', 'mapping'] as const) {
  test(`${kind} name keeps composition text and cancels IME Escape only`, async ({ page }) => {
    await openProvider(page);
    let label: string;
    if (kind === 'mapping') {
      await page.getByRole('tab', { name: /^模型/ }).click();
      await page.getByRole('button', { name: '手动添加映射', exact: true }).click();
      label = 'Anchor 公开别名';
    } else {
      await page.getByRole('button', { name: '更多操作', exact: true }).click();
      await page.getByRole('menuitem', { name: kind === 'rename' ? '改名' : '复制为新供应商', exact: true }).click();
      label = kind === 'rename' ? '新名称' : '新供应商名称';
    }
    const input = page.getByLabel(label, { exact: true });
    await input.fill('');
    await input.focus();
    const session = await page.context().newCDPSession(page);
    await session.send('Input.imeSetComposition', { text: 'zhong ', selectionStart: 6, selectionEnd: 6 });
    await expect(input).toHaveValue('zhong ');
    expect(await composingKey(input, 'Escape', { isComposing: true })).toBe(true);
    await expect(page.getByRole('dialog')).toBeVisible();
    await session.send('Input.insertText', { text: '中文名称' });
    await expect(input).toHaveValue('中文名称');
    expect(await composingKey(input, 'Escape', { keyCode: 229 })).toBe(true);
    await expect(page.getByRole('dialog')).toBeVisible();
    await input.press('Escape');
    await expect(page.getByRole('dialog')).toBeHidden();
    await session.detach();
  });
}
