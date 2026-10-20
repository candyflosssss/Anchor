import { describe, expect, it } from 'vitest';
import { DesktopProblem } from '../../desktop/transport';
import { remoteUpdateMessage } from './remote-update-errors.model';

describe('SSH update diagnostics', () => {
  it('explains the old remote CLI prerequisite without blaming a proxy', () => {
    const message = remoteUpdateMessage(new DesktopProblem('remote_anchor_unsupported'), 'checking');
    expect(message).toContain('被控机 Anchor CLI 版本不支持');
    expect(message).toContain('desktop.57');
    expect(message).toContain('未开始更新');
    expect(message).not.toMatch(/代理|Windows|PAC/);
  });
  it.each([
    ['ssh_auth', 'SSH 认证失败'],
    ['ssh_host_key', '主机密钥校验失败'],
    ['ssh_unreachable', '无法通过 SSH 连接'],
    ['remote_anchor_missing', '找不到被控机 Anchor CLI'],
    ['identity', '被控机实例身份'],
    ['unauthorized', '被控机管理授权失效'],
  ] as const)('retains the %s diagnosis and remote target', (code, text) => {
    const message = remoteUpdateMessage(new DesktopProblem(code), 'checking');
    expect(message).toContain(text);
    expect(message).not.toMatch(/Windows|PAC|连接更新源或下载失败/);
  });
  it.each(['remote_anchor', 'read_error', 'unknown'] as const)('does not invent a network cause for %s during discovery', code => {
    const message = remoteUpdateMessage(new DesktopProblem(code), 'checking');
    expect(message).toContain('尚未开始更新');
    expect(message).toContain('非交互 SSH');
    expect(message).toContain('本机代理不会自动传给被控机');
    expect(message).not.toMatch(/连接更新源或下载失败|install.log/);
  });
  it('does not promise an unchanged remote program after an unknown mutation', () => {
    const message = remoteUpdateMessage(new DesktopProblem('unknown'), 'installing');
    expect(message).toContain('结果未能确认');
    expect(message).toContain('不要反复提交');
    expect(message).not.toMatch(/未开始更新|未被覆盖|未执行切换/);
  });
  it('never includes untrusted remote error text', () => {
    const message = remoteUpdateMessage(new Error('synthetic-secret terminal-output'), 'checking');
    expect(message).not.toContain('synthetic-secret');
    expect(message).toContain('尚未开始更新');
  });
});
