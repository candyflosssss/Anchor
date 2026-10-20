import { DesktopProblem } from '../../desktop/transport';

export function remoteUpdateMessage(error: unknown, operation: 'checking' | 'installing'): string {
  const code = error instanceof DesktopProblem ? error.code : 'unknown';
  const messages: Record<string, string> = {
    remote_anchor_unsupported: '被控机 Anchor CLI 版本不支持在线更新。请先将被控机 CLI 升级到 desktop.57 或更高版本，再使用此入口。本次未开始更新。',
    unsupported: '被控机版本不支持此更新操作，请先升级被控机 CLI。',
    remote_anchor_missing: '找不到被控机 Anchor CLI。请检查该设备配置的程序路径。',
    ssh_missing: '本机缺少 SSH 客户端，无法连接被控机。',
    ssh_auth: 'SSH 认证失败。请在设备连接中重新连接被控机。',
    ssh_host_key: '被控机 SSH 主机密钥校验失败。请在设备连接中核对主机身份。',
    ssh_unreachable: '无法通过 SSH 连接被控机。请检查设备网络后重新连接。',
    offline: '被控机管理连接已断开。请在设备连接中重新连接。',
    closed: '被控机连接已关闭。请在设备连接中重新连接。',
    tunnel_failed: '被控机管理隧道不可用。请在设备连接中重新连接。',
    remote_control_port: '被控机管理端口与已保存的设备配置不一致，已取消操作。',
    remote_core_not_ready: '被控机核心尚未就绪。请先检查该设备状态。',
    identity: '被控机实例身份与当前设备不一致，已取消操作。请重新连接并核对设备。',
    unauthorized: '被控机管理授权失效，已取消操作。请重新连接该设备。',
    invalid: '被控机更新源、渠道或候选版本无效。请检查被控机的更新配置后重新检查。',
    integrity: '被控机更新文件大小或 SHA256 校验失败，未执行切换。请重新检查更新。',
    permission: '被控机无法写入更新目录或启动更新程序。请检查远端用户权限。',
    conflict: '被控机正忙或状态已变化。请等待当前任务结束后重新检查。',
  };
  if (messages[code]) return messages[code];
  if (operation === 'installing') return '被控机更新结果未能确认。请重新连接并检查远端版本、核心状态及 updates/install.log，不要反复提交更新。';
  return '被控机未能完成更新检查，尚未开始更新。请核对远端 Anchor CLI 版本、更新源及网络。检查由被控机通过非交互 SSH 执行，本机代理不会自动传给被控机。';
}
