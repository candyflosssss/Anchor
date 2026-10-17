import { DeviceSwitcher } from '../features/connections/device-switcher.view';
import { useDeviceSwitcher } from '../features/connections/use-device-switcher';
import { ChevronDown, LogOut, Power, RotateCw } from 'lucide-react';
import { Button } from '../components/ui/button';
import { Popover } from '../components/shared/popover-menu';
import { prototype } from '../desktop/environment';
import type { useDesktopSession } from '../desktop/use-desktop-session';
import { cn } from '../lib/utils';

type Session = ReturnType<typeof useDesktopSession>;

type Props = { session: Session; busy: boolean; onExit: () => void; onStop: () => void; onRestart: () => void; onManage: () => void; onActivity: (busy: boolean) => void };

/** Device selection is primary; lifecycle actions remain a separate footer. */
export function CoreStatus(props: Props) {
  if (prototype) return <span className="connection-label"><span className="mini-dot" />本机实例 · 模拟，未连接</span>;
  return <DesktopCoreStatus {...props} />;
}
function DesktopCoreStatus({ session, busy, onExit, onStop, onRestart, onManage, onActivity }: Props) {
  const switcher = useDeviceSwitcher(session.select, onActivity);
  const blocked = busy || session.stopping || session.restarting || session.phase === 'starting';
  const target = session.info ?? session.target;
  const name = target?.transport === 'ssh' ? target.profile_name : '本机实例';
  const label = session.restarting ? `${name} · 重启中` : session.phase === 'starting' || switcher.pending ? `${name} · 连接中` : session.phase === 'ready' ? `${name} · 已连接` : `${name} · 未连接`;
  const info = session.info;
  return <Popover label="切换设备" className="core-popover !w-[min(340px,calc(100vw-16px))] max-h-[calc(100dvh-64px)] overflow-y-auto" trigger={props => <button type="button" className={cn('connection-label core-trigger', session.phase === 'ready' && 'is-ready')} {...props}>
    <span className="mini-dot" /><span>{label}</span><ChevronDown size={13} aria-hidden="true" />
  </button>}>
    {close => <div className="core-panel">
      <DeviceSwitcher controller={switcher} current={target?.profile_name} connected={session.phase === 'ready'} disabled={blocked} onManage={onManage} close={close} />
      <div className="core-actions flex-wrap border-t pt-2">
        <Button variant="ghost" size="sm" disabled={blocked || !!switcher.pending} onClick={() => { close(); onExit(); }}><LogOut />退出界面</Button>
        {target && <Button variant="outline" size="sm" disabled={blocked || !!switcher.pending || session.error === 'unknown' || (target.transport === 'ssh' && session.phase !== 'ready')} title={target.transport === 'ssh' && session.phase !== 'ready' ? '请先重新连接远程设备，再重启核心' : undefined} onClick={() => { close(); onRestart(); }}><RotateCw />{session.phase === 'ready' || target.transport === 'ssh' ? '重启核心' : '恢复本机核心'}</Button>}
        {info?.can_stop && <Button variant="outline" size="sm" disabled={blocked || !!switcher.pending} onClick={() => { close(); onStop(); }}><Power />停止核心</Button>}
      </div>
    </div>}
  </Popover>;
}
