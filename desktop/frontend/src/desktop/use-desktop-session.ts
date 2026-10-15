import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProblemCode, SessionInfo } from './contract.generated';
import { DesktopProblem, desktopSession } from './transport';
import { prototype } from './environment';
export const desktopMessages: Record<ProblemCode, string> = {
  agent_operation_failed: 'Agent 托管操作未完成，请核对当前状态和本机恢复记录。',
  agent_model_unavailable: 'Agent 默认模型不可用，请检查模型映射和候选供应商。',
  agent_catalog_invalid: '无法生成 Agent 模型目录，请检查模板和模型元数据。',
  agent_config_conflict: 'Agent 托管配置已被外部修改，已保留这些修改。',
  agent_config_unsafe: 'Agent 配置无法安全修改，请检查结构、文件类型与权限。',
  referenced: '项目仍有引用，无法删除。',
  offline: '当前设备核心不可用，请重新连接。',
  unauthorized: '管理凭据不可用，请重新连接设备以完成 SSH 鉴权，或检查本机实例权限。',
  identity: '实例身份或接口版本不匹配。未连接、覆盖或停止该进程。',
  startup_failed: '核心启动失败或未在期限内就绪。请检查运行时依赖、目录权限和监听端口；不会强杀占用进程。',
  unsupported: '桌面接口不可用或版本不兼容。请使用正式桌面入口，浏览器不能操作本机核心。',
  read_error: '无法读取本机实例状态。请检查后重新连接。',
  unknown: '停服请求结果未知。请先检查核心状态，不要重复提交。',
  invalid: '操作未通过校验，未提交变更。',
  conflict: '配置已发生变更，请重新读取。',
  closed: '桌面会话已关闭。',
  integrity: '更新文件校验失败，未安装。',
  ssh_missing: '找不到系统 OpenSSH 客户端，请安装 OpenSSH 后重试。',
  ssh_host_key: '主机密钥尚未信任或已变化。首次连接可勾选信任首次主机密钥；已有密钥变化时请核实目标机器。',
  ssh_auth: 'SSH 鉴权失败，请检查用户名、密码、密钥或 SSH Agent。已保存的密码连接需重新填写本次密码。',
  ssh_unreachable: '无法访问 SSH 目标，请检查地址、端口和网络。',
  remote_anchor: '远端连接引导失败或返回数据无效，请核对远端 Anchor 路径及管理端口。',
  remote_anchor_missing: 'SSH 已登录，但该账号下找不到可执行的 Anchor。已检查 PATH、~/.local/bin 和 ~/bin；请安装 Anchor，或在高级选项填写正确路径。',
  remote_anchor_unsupported: '已找到远端程序，但它不支持设备连接引导。请将远端 Anchor CLI 更新到支持 context bootstrap 的版本，并确认路径指向 CLI。',
  remote_control_port: '远端 Anchor 已配置的管理端口与本次连接不一致。请在高级选项填写远端实际管理端口。',
  remote_core_not_ready: '已找到支持连接引导的远端 Anchor，但核心启动或就绪检查失败。请检查远端端口占用及核心状态后重试。',
  tunnel_failed: 'SSH 管理隧道未建立或已断开，请重新连接。',
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
  }, [phase, info?.instance_id, info?.profile_name]);
  async function stop() {
    if (!info || stopLock.current) return;
    stopLock.current = true; setStopping(true); generation.current++;
    try { await desktopSession.stop(info); if (active.current) { setInfo(null); setPhase('stopped'); setError(null); } }
    catch (e) { if (active.current) { setInfo(null); setError(e instanceof DesktopProblem ? e.code : 'unknown'); setPhase('error'); } }
    finally { stopLock.current = false; if (active.current) setStopping(false); }
  }
  const select = useCallback((next: SessionInfo) => { generation.current++; setInfo(next); setPhase('ready'); setError(null); }, []);
  return { info, phase, error, stopping, connect, stop, select };
}
