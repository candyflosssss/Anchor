import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import incomplete from '../../../../contract/codex-settings-incomplete.json';
import fixture from '../../../../contract/codex-settings.json';
import { checkedCodexSettings } from './codex-settings.model';
import { CodexCompactionStatus } from './codex-settings.view';

describe('per-model compaction status', () => {
  it('shows missing context as an alert even when sync has no failures', () => {
    const html = renderToStaticMarkup(createElement(CodexCompactionStatus, { state: checkedCodexSettings(incomplete) }));
    expect(html).toContain('unknown（默认模型）');
    expect(html).toContain('role="alert"');
    expect(html).toContain('未配置压缩阈值：缺少已保存的上下文窗口');
    expect(html).toContain('272,000');
    expect(html).toContain('244,800');
    expect(html).toContain('需重启 Codex 加载');
    expect(html).toContain('是否实际触发压缩，尚未验证');
    expect(html).not.toContain('已生效');
  });
  it('distinguishes external drift, unapplied targets and an empty target list', () => {
    for (const [status, text] of [['needs_sync', '配置待同步'], ['unmanaged', '尚未应用托管配置'], ['unsupported', '没有模型目录']] as const) {
      const state = checkedCodexSettings({ ...fixture, compaction: [{ ...incomplete.compaction[1], status }] });
      const html = renderToStaticMarkup(createElement(CodexCompactionStatus, { state }));
      expect(html).toContain(text); expect(html).toContain('role="alert"');
    }
    expect(renderToStaticMarkup(createElement(CodexCompactionStatus, { state: checkedCodexSettings(fixture) }))).toContain('自动压缩尚未配置');
  });
});
