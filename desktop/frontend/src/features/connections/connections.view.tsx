import { identifierPattern } from '../../lib/identifier';
import { Monitor, Server, LoaderCircle } from 'lucide-react';
import type { SessionInfo } from '../../desktop/contract.generated';
import { desktopMessages } from '../../desktop/use-desktop-session';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { useConnectionController } from './use-connection-controller';
import './connections.css';

export function ConnectionsView({ currentProfile, disabled, onConnected, onActivity }: { currentProfile?: string; disabled?: boolean; onConnected: (info: SessionInfo) => void; onActivity: (busy: boolean) => void }) {
  const c = useConnectionController(onConnected, onActivity);
  const busy = !!disabled || c.pending !== null;
  const profiles = c.profiles.map(profile => ({ ...profile, active: profile.name === currentProfile }));
  return <main className="workspace connection-workspace">
    <div className="page-header"><div><h1>设备连接</h1><p className="text-muted-foreground">连接其他机器上的 Anchor，在这里管理它的供应商、模型与 Agent。</p></div><Button variant="outline" disabled={busy || c.loading} onClick={() => { void c.refresh(); }}>刷新</Button></div>
    {c.error && <p role="alert" className="connection-error">{desktopMessages[c.error]}</p>}
    <section aria-label="已保存的设备" className="connection-profiles">
      {c.loading && <p role="status">正在读取连接档案…</p>}
      {profiles.map(profile => <article key={profile.name} className="connection-profile">
        {profile.transport === 'local' ? <Monitor size={20} /> : <Server size={20} />}
        <div className="min-w-0 flex-1"><strong>{profile.transport === 'local' ? '本机' : profile.name}</strong><p className="text-muted-foreground break-all">{profile.transport === 'local' ? '本机 Anchor 核心' : profile.target}</p>{profile.instance_id && <p className="text-xs text-muted-foreground break-all">实例 {profile.instance_id}</p>}</div>
        {profile.authentication === 'password' && <label className="text-xs">SSH 密码<Input type="password" autoComplete="off" aria-label={`${profile.name} 的 SSH 密码`} disabled={busy} value={c.passwords[profile.name] ?? ''} onChange={e => c.setPasswords(p => ({ ...p, [profile.name]: e.target.value }))} placeholder={profile.active ? '当前连接可直接复用' : '本次登录使用，不保存'} /></label>}
        {profile.active && <span className="text-xs text-muted-foreground">当前设备</span>}
        <Button variant="outline" disabled={busy} onClick={() => { void c.connect(profile.name); }}>{c.pending === profile.name && <LoaderCircle className="animate-spin" />}{profile.active ? '重新连接' : '连接'}</Button>
      </article>)}
    </section>
    <section className="connection-create" aria-labelledby="ssh-heading">
      <h2 id="ssh-heading">添加远程设备</h2>
      <p className="text-muted-foreground">使用 SSH 密码、密钥或已有 SSH 配置连接。Anchor 会完成鉴权、核心启动和管理隧道连接，关闭界面后远端核心继续运行。</p>
      <form onSubmit={event => { event.preventDefault(); void c.connect(); }}>
        <fieldset disabled={busy} className="connection-fields">
          <label>连接名称<Input required pattern={identifierPattern} placeholder="开发服务器" value={c.draft.name} onChange={e => c.setDraft({ ...c.draft, name: e.target.value })} /><small>1–128 个中文／字母／数字或 ._-，以字母或数字开头</small></label>
          <label>SSH 目标<Input required placeholder="user@192.168.1.20 或 SSH 别名" value={c.draft.target} onChange={e => c.setDraft({ ...c.draft, target: e.target.value })} /></label>
          <label className="connection-trust"><input type="checkbox" checked={c.usePassword} onChange={e => { c.setUsePassword(e.target.checked); c.setDraft({ ...c.draft, password: '' }); }} /><span>使用密码登录</span></label>
          {c.usePassword && <label>SSH 密码<Input aria-label="SSH 密码" type="password" required autoComplete="off" value={c.draft.password} onChange={e => c.setDraft({ ...c.draft, password: e.target.value })} /><small>只用于本次登录，不保存；密码模式使用 user@主机地址，不读取 SSH 别名。</small></label>}
          <details><summary>连接选项</summary><div className="connection-advanced">
            <label>SSH 端口<Input type="number" min="1" max="65535" placeholder="使用 SSH 配置，通常为 22" value={c.draft.port || ''} onChange={e => c.setDraft({ ...c.draft, port: Number(e.target.value) })} /></label>
            <label>私钥文件路径（可选）<Input placeholder="留空使用 SSH Agent 或默认密钥" value={c.draft.identity_file} onChange={e => c.setDraft({ ...c.draft, identity_file: e.target.value })} /></label>
            <label>远端 Anchor 路径<Input required value={c.draft.remote_executable} onChange={e => c.setDraft({ ...c.draft, remote_executable: e.target.value })} /></label>
            <label>远端管理端口<Input required type="number" min="1" max="65535" value={c.draft.remote_control_port} onChange={e => c.setDraft({ ...c.draft, remote_control_port: Number(e.target.value) })} /></label>
          </div></details>
          <label className="connection-trust"><input type="checkbox" checked={c.draft.trust_new_host} onChange={e => c.setDraft({ ...c.draft, trust_new_host: e.target.checked })} /><span>信任首次连接的主机密钥<small>首次信任后会记录主机密钥；已保存的密钥变化时始终拒绝连接。</small></span></label>
          <p className="text-sm text-muted-foreground">远端需已安装支持设备连接的 Anchor 并开启 SSH。密码只用于本次登录；密钥模式使用系统 OpenSSH，支持 SSH 配置与 SSH Agent。</p>
          <Button type="submit">{c.pending === 'new' && <LoaderCircle className="animate-spin" />}{c.pending === 'new' ? '正在连接…' : '添加并连接'}</Button>
        </fieldset>
      </form>
    </section>
  </main>;
}
