import { ImeForm } from '../../components/shared/ime-form';
import { useEffect, useState } from 'react';
import { Check, LoaderCircle, Monitor, Server, Settings2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { desktopMessages } from '../../desktop/use-desktop-session';
import type { useDeviceSwitcher } from './use-device-switcher';
import type { ConnectionProfile } from './connection.model';
import { cn } from '../../lib/utils';

type Switcher = ReturnType<typeof useDeviceSwitcher>;
export function DeviceSwitcher({ controller: c, current, connected, version, disabled, onManage, close }: {
  controller: Switcher; current?: string; connected: boolean; version?: string; disabled: boolean; onManage: () => void; close: () => void;
}) {
  const [passwordProfile, setPasswordProfile] = useState<ConnectionProfile | null>(null);
  const [password, setPassword] = useState('');
  const [remember, setRemember] = useState(false);
  const { refresh } = c;
  useEffect(() => { void refresh(); }, [refresh]);
  async function connect(name: string, secret = '', save = false) {
    setPassword('');
    setRemember(false);
    if (await c.connect(name, secret, save)) close();
  }
  function select(profile: ConnectionProfile) {
    setPassword(''); setRemember(false); setPasswordProfile(null);
    if (profile.name === current && connected) return;
    if (profile.authentication === 'password') {
      setPasswordProfile(profile);
      if (profile.password_saved) void connect(profile.name);
    }
    else void connect(profile.name);
  }
  return <>
    <strong>切换设备</strong>
    {c.loading && <p role="status">正在读取设备…</p>}
    <div role="group" aria-label="可切换的设备" className="grid max-h-64 gap-1 overflow-y-auto">
      {c.profiles.map(profile => {
        const active = profile.name === current && connected;
        return <button type="button" key={profile.name} aria-label={`切换到${profile.transport === 'local' ? '本机' : profile.name}`} aria-pressed={active} disabled={disabled || !!c.pending || c.loading}
          className={cn('flex min-h-12 w-full items-center gap-3 rounded-md px-2 py-2 text-left text-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50', active && 'bg-accent')}
          onClick={() => select(profile)}>
          {profile.transport === 'local' ? <Monitor size={16} className="shrink-0" /> : <Server size={16} className="shrink-0" />}
          <span className="min-w-0 flex-1"><span className="block truncate font-medium">{profile.transport === 'local' ? '本机' : profile.name}</span><span className="block truncate text-xs text-muted-foreground">{profile.transport === 'local' ? '本机 Anchor 核心' : profile.target}</span>{active && <span className="block break-all text-xs text-muted-foreground">核心版本：{version || '未提供'}</span>}</span>
          {c.pending === profile.name ? <LoaderCircle size={16} className="shrink-0 animate-spin motion-reduce:animate-none" /> : active && <><span className="text-xs text-muted-foreground">当前</span><Check size={16} className="shrink-0" /></>}
        </button>;
      })}
    </div>
    {!c.loading && !c.profiles.length && !c.error && <p>暂无已保存设备，请前往设备连接添加。</p>}
    {passwordProfile && <ImeForm className="grid gap-2 rounded-md border p-3" onSubmit={event => { event.preventDefault(); void connect(passwordProfile.name, password, remember && !!password); }}>
      <label className="grid gap-2">{passwordProfile.name} 的 SSH 密码<Input type="password" autoComplete="off" autoFocus required={!passwordProfile.password_saved} placeholder={passwordProfile.password_saved ? '留空使用已保存密码' : ''} disabled={disabled || !!c.pending} value={password} onChange={e => setPassword(e.target.value)} /></label>
      {typeof passwordProfile.password_saved === 'boolean' && <label className="flex items-center gap-2"><input type="checkbox" checked={remember} disabled={disabled || !!c.pending || !password} onChange={e => setRemember(e.target.checked)} />记住密码（系统凭据库）</label>}
      <p>{passwordProfile.password_saved ? '已保存密码留在系统凭据库，输入新密码可用于本次连接。' : '未勾选记住密码时仅本次使用，不保存。'}</p>
      <div className="flex justify-end gap-2"><Button type="button" size="sm" variant="ghost" disabled={!!c.pending} onClick={() => { setPassword(''); setRemember(false); setPasswordProfile(null); }}>取消</Button><Button type="submit" size="sm" disabled={disabled || !!c.pending || (!password && !passwordProfile.password_saved)}>连接设备</Button></div>
    </ImeForm>}
    {c.pending && <p role="status">正在连接设备…</p>}
    {c.notice && <p role="status">{c.notice}</p>}
    {c.error && <div role="alert" className="grid gap-2 text-destructive"><p>{desktopMessages[c.error]}</p><Button size="sm" variant="outline" disabled={disabled || !!c.pending} onClick={() => { void c.refresh(); }}>刷新设备列表</Button></div>}
    <Button variant="ghost" className="justify-start" disabled={disabled || !!c.pending} onClick={() => { close(); onManage(); }}><Settings2 />管理设备连接</Button>
  </>;
}
