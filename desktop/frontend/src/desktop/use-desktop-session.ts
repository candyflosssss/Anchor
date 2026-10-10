import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProblemCode, SessionInfo } from './contract.generated';
import { DesktopProblem, desktopSession } from './transport';
import { prototype } from './environment';
export const desktopMessages: Record<ProblemCode, string> = {
  offline: '本机核心不可用。可重新连接；不会切换到其他实例。',
  unauthorized: '管理凭据不可用。请检查本机实例权限，不会要求在页面填写管理令牌。',
  identity: '实例身份或接口版本不匹配。未连接、覆盖或停止该进程。',
  startup_failed: '核心启动失败或未在期限内就绪。请检查运行时依赖、目录权限和监听端口；不会强杀占用进程。',
  unsupported: '桌面接口不可用或版本不兼容。请使用正式桌面入口，浏览器不能操作本机核心。',
  read_error: '无法读取本机实例状态。请检查后重新连接。',
  unknown: '停服请求结果未知。请先检查核心状态，不要重复提交。',
  invalid: '操作未通过校验，未提交变更。',
  conflict: '配置已发生变更，请重新读取。',
  closed: '桌面会话已关闭。',
  integrity: '更新文件校验失败，未安装。',
  permission: '本机文件或安装器权限不足。',
};
export function useDesktopSession() {
  const [info, setInfo] = useState<SessionInfo | null>(null);
  const [phase, setPhase] = useState<'starting' | 'ready' | 'error' | 'stopped'>(prototype ? 'ready' : 'starting');
  const [error, setError] = useState<ProblemCode | null>(null);
  const [stopping, setStopping] = useState(false);
  const active = useRef(true);
  const generation = useRef(0);
  const starting = useRef<Promise<SessionInfo> | null>(null);
  const stopLock = useRef(false);
  const connect = useCallback(async () => {
    const epoch = ++generation.current;
    setPhase('starting'); setError(null);
    starting.current ??= desktopSession.start();
    const request = starting.current;
    try { const next = await request; if (active.current && epoch === generation.current) { setInfo(next); setPhase('ready'); } }
    catch (e) { if (active.current && epoch === generation.current) { setInfo(null); setError(e instanceof DesktopProblem ? e.code : 'startup_failed'); setPhase('error'); } }
    finally { if (starting.current === request) starting.current = null; }
  }, []);
  useEffect(() => {
    active.current = true;
    if (!prototype) void connect();
    return () => { active.current = false; };
  }, [connect]);
  useEffect(() => {
    if (prototype || phase !== 'ready') return;
    let canceled = false, pending = false;
    const epoch = generation.current;
    const interval = window.setInterval(async () => {
      if (pending || stopLock.current) return;
      pending = true;
      try { const next = await desktopSession.status(); if (!canceled && active.current && generation.current === epoch) setInfo(next); }
      catch (e) { if (!canceled && active.current && generation.current === epoch) { setInfo(null); setError(e instanceof DesktopProblem ? e.code : 'offline'); setPhase('error'); } }
      finally { pending = false; }
    }, 5000);
    return () => { canceled = true; window.clearInterval(interval); };
  }, [phase]);
  async function stop() {
    if (!info || stopLock.current) return;
    stopLock.current = true; setStopping(true); generation.current++;
    try { await desktopSession.stop(info); if (active.current) { setInfo(null); setPhase('stopped'); setError(null); } }
    catch (e) { if (active.current) { setInfo(null); setError(e instanceof DesktopProblem ? e.code : 'unknown'); setPhase('error'); } }
    finally { stopLock.current = false; if (active.current) setStopping(false); }
  }
  return { info, phase, error, stopping, connect, stop };
}
