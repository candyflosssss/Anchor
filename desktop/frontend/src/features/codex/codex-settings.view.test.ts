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
  it('does not claim prompt restoration when opt-out synchronization failed', () => {
    const state = checkedCodexSettings({ ...fixture, needs_sync: ['codex-work'] });
    const html = renderToStaticMarkup(createElement(CodexCompactionStatus, { state }));
    expect(html).toContain('尚未恢复原配置');
    expect(html).toContain('role="alert"');
    expect(html).not.toContain('保留 Codex 自身设置');
  });
  it('distinguishes external drift, unapplied targets and an empty target list', () => {
    for (const [status, text] of [['needs_sync', '配置待同步'], ['unmanaged', '尚未应用托管配置'], ['unsupported', '没有模型目录']] as const) {
      const state = checkedCodexSettings({ ...fixture, settings: { ...fixture.settings, override_model_catalog: true }, compaction: [{ ...incomplete.compaction[1], status }] });
      const html = renderToStaticMarkup(createElement(CodexCompactionStatus, { state }));
      expect(html).toContain(text); expect(html).toContain('role="alert"');
    }
    expect(renderToStaticMarkup(createElement(CodexCompactionStatus, { state: checkedCodexSettings(fixture) }))).toContain('提示词与模型目录覆盖已关闭');
  });
});
