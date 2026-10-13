import { ChevronDown, LogOut, Power } from 'lucide-react';
import { Button } from '../components/ui/button';
import { Popover } from '../components/shared/popover-menu';
import { prototype } from '../desktop/environment';
import type { useDesktopSession } from '../desktop/use-desktop-session';
import { cn } from '../lib/utils';

type Session = ReturnType<typeof useDesktopSession>;

/** Header chip for the local core. Lifecycle actions (exit UI / stop core) live in its popover instead of a permanent bar. */
export function CoreStatus({ session, busy, onExit, onStop }: { session: Session; busy: boolean; onExit: () => void; onStop: () => void }) {
  if (prototype) return <span className="connection-label"><span className="mini-dot" />本机实例 · 模拟，未连接</span>;
  const label = session.phase === 'ready' ? '本机实例 · 已连接' : session.phase === 'starting' ? '本机实例 · 启动中' : '本机实例 · 未连接';
  const info = session.info;
  return <Popover label="本机核心" className="core-popover" trigger={props => <button type="button" className={cn('connection-label core-trigger', session.phase === 'ready' && 'is-ready')} {...props}>
    <span className="mini-dot" /><span>{label}</span><ChevronDown size={13} aria-hidden="true" />
  </button>}>
    {close => <div className="core-panel">
      <strong>本机核心</strong>
      {info && <p>{info.source === 'started' ? '本次启动' : '复用已有核心'}</p>}
      {info && <p className="mono break-all">实例 {info.instance_id}</p>}
      <div className="core-actions">
        <Button variant="ghost" size="sm" disabled={busy || session.stopping} onClick={() => { close(); onExit(); }}><LogOut />退出界面</Button>
        {info?.can_stop && <Button variant="outline" size="sm" disabled={busy || session.stopping} onClick={() => { close(); onStop(); }}><Power />停止核心</Button>}
      </div>
    </div>}
  </Popover>;
}
