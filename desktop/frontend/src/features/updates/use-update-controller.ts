import { useEffect, useRef, useState } from 'react';
import type { UpdateInfo } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { updates, type Channel } from './update.model';

export function useUpdateController(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [info, setInfo] = useState<UpdateInfo | null>(null);
  const [channel, setChannel] = useState<Channel>('alpha');
  const [operation, setOperation] = useState('');
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState('');
  const [confirm, setConfirm] = useState(false);
  const alive = useRef(false);
  const latch = useRef(false);
  const generation = useRef(0);
  useEffect(() => {
    alive.current = true;
    const epoch = ++generation.current;
    if (enabled) void updates.status().then(value => {
      if (alive.current && epoch === generation.current) { setInfo(value); setChannel(value.channel as Channel); }
    }).catch(e => { if (alive.current && epoch === generation.current) setError(message(e)); })
      .finally(() => { if (alive.current && epoch === generation.current) setLoading(false); });
    return () => { alive.current = false; };
  }, [enabled]);
  const busy = loading || operation !== '' || info?.phase === 'installing';
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function run(name: string, action: () => Promise<UpdateInfo>) {
    if (!enabled || busy || latch.current) return;
    const epoch = ++generation.current;
    latch.current = true; setOperation(name); setError('');
    try { const result = await action(); if (alive.current && epoch === generation.current) setInfo(result); }
    catch (e) { if (alive.current && epoch === generation.current) setError(message(e)); }
    finally { latch.current = false; if (alive.current && epoch === generation.current) { setOperation(''); setConfirm(false); } }
  }
  function selectChannel(value: Channel) {
    if (busy) return;
    setChannel(value); setError(''); setConfirm(false);
    setInfo(current => current && { ...current, channel: value, latest_version: '',
      available: false, phase: 'idle', download_path: '', install_kind: '' });
  }
  return { info, channel, setChannel: selectChannel, operation, busy, error, confirm, setConfirm,
    check: () => run('checking', () => updates.check(channel)),
    download: () => info && run('downloading', () => updates.download(info.latest_version)),
    install: () => info && run('installing', () => updates.install(info.latest_version)) };
}
function message(error: unknown): string {
  const code = error instanceof DesktopProblem ? error.code : 'unknown';
  const messages: Record<string, string> = {
    integrity: '安装包大小或 SHA256 校验失败。未执行安装，请重新检查并下载。',
    invalid: '更新源、渠道或确认版本无效。请重新检查；自定义更新源由 CLI 配置。',
    conflict: '更新任务或本机核心仍在使用中。不会强杀进程，请等待退出后重试。',
    permission: '无法写入下载目录或启动安装器。请检查当前用户权限。',
    unsupported: '没有兼容安装包，或核心不支持正常停服。不会强制替换旧版。',
    identity: '本机核心身份不匹配，已取消更新。',
    unauthorized: '无法取得本机核心管理授权，已取消安装。',
    unknown: '更新操作结果未能确认。请检查下载状态及 updates/install.log，不要反复安装。',
  };
  return messages[code] ?? '读取更新源或下载失败。请检查网络、GitHub 限流及更新源配置后重试；应用会使用环境变量或 Windows 系统代理（不支持 PAC 自动配置脚本）。现有程序未被覆盖。';
}
