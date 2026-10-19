import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { KeyCooldownSettings } from './key-cooldown.view';

describe('compact Key cooldown menu', () => {
  it('only renders names, values and controls without explanatory prose', () => {
    const html = renderToStaticMarkup(createElement(KeyCooldownSettings, { coreReady: true, onActivity: () => {} }));
    for (const text of ['Key 自动降权', '自动降权', '降权后权重（%）', '连续失败次数', '降权时长（秒）', '429 降权时长（秒）', '保存', '读取']) expect(html).toContain(text);
    for (const text of ['card-hint', 'Tooltip', 'Retry-After', '默认', '表示', '适用于', '修改后', '<p']) expect(html).not.toContain(text);
    expect(html).toContain('role="switch"');
    expect(html).toContain('min="1" max="100"');
    expect(html).toContain('min="1" max="3600"');
  });
  it('shows an explicit disconnected state without pretending settings are available', () => {
    const html = renderToStaticMarkup(createElement(KeyCooldownSettings, { coreReady: false, onActivity: () => {} }));
    expect(html).toContain('未连接'); expect(html).not.toContain('<form');
  });
});
