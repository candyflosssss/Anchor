import { expect, test } from '@playwright/test';

test('inline platform account form does not save on candidate Enter', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: '账户与余额', exact: true }).click();
  await page.getByText('账户接口地址（可选）', { exact: true }).click();
  const input = page.getByLabel('站点根地址', { exact: true });
  await input.fill('https://example.invalid/中文路径');
  await input.evaluate(node => {
    const form = (node as HTMLInputElement).form!;
    form.dataset.submits = '0';
    form.addEventListener('submit', event => {
      event.preventDefault(); event.stopImmediatePropagation();
      form.dataset.submits = String(Number(form.dataset.submits) + 1);
    });
  });
  await input.dispatchEvent('compositionstart');
  await input.press('Enter');
  await expect(page.locator('.account-form')).toHaveAttribute('data-submits', '0');
  await input.dispatchEvent('compositionend');
  await input.press('Enter');
  await expect(page.locator('.account-form')).toHaveAttribute('data-submits', '1');
});

test('supplier search preserves real Chinese composition and remains focused', async ({ page }) => {
  await page.goto('/');
  const input = page.getByRole('textbox', { name: '搜索供应商名称或地址' });
  await input.focus();
  const session = await page.context().newCDPSession(page);
  await session.send('Input.imeSetComposition', { text: 'moyu ', selectionStart: 5, selectionEnd: 5 });
  await expect(input).toHaveValue('moyu ');
  await expect(input).toBeFocused();
  await session.send('Input.insertText', { text: '摸鱼' });
  await expect(input).toHaveValue('摸鱼');
  await expect(input).toBeFocused();
  await expect(page.getByRole('button', { name: '摸鱼AI', exact: true })).toBeVisible();
  await session.detach();
});

test('model directory search keeps Chinese composition when filtering empties the results', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '摸鱼AI', exact: true }).click();
  await page.getByRole('tab', { name: /^模型/ }).click();
  await page.getByRole('button', { name: '获取模型目录', exact: true }).click();
  const input = page.getByRole('searchbox', { name: '搜索模型目录' });
  await input.focus();
  const session = await page.context().newCDPSession(page);
  await session.send('Input.imeSetComposition', { text: 'moxing ', selectionStart: 7, selectionEnd: 7 });
  await expect(input).toHaveValue('moxing ');
  await expect(input).toBeFocused();
  await session.send('Input.insertText', { text: '中文模型' });
  await expect(input).toHaveValue('中文模型');
  await expect(input).toBeFocused();
  await expect(page.getByText('没有匹配的模型', { exact: true })).toBeVisible();
  await session.detach();
});
