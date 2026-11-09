import { RefreshCw } from 'lucide-react';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { prototype } from '../../desktop/environment';
import { useAutostart } from './use-autostart';

export function AutostartSettings({ onActivity }: { onActivity: (busy: boolean) => void }) {
  const c = useAutostart(!prototype, onActivity);
  const disabled = c.busy || c.needsRead || c.unsupported || !c.state?.supported;
  return <section className="detail-card" aria-labelledby="autostart-title">
    <header className="card-toolbar"><h2 id="autostart-title">本机登录自启</h2>
      {!prototype && <Button variant="ghost" size="icon" title="重新读取登录项" aria-label="重新读取登录项" disabled={c.busy} onClick={() => void c.read()}><RefreshCw size={16} /></Button>}
    </header>
    {prototype ? <p role="status">桌面专用</p> : <>
      <label className="model-setting-field"><span>登录 Windows 后启动后台核心</span><Switch aria-label="登录 Windows 后启动后台核心" checked={c.state?.enabled ?? false} disabled={disabled || (!c.state?.enabled && !c.state?.can_enable)} onCheckedChange={c.setPending} /></label>
      {c.state && !c.state.supported && <p role="status">当前平台不支持</p>}
      {c.unsupported && <p role="status">当前宿主版本不支持</p>}
      {c.state?.supported && !c.state.can_enable && <p role="status">本机核心尚未初始化，或桌面启动器不可用。</p>}
      {c.state?.enabled && !c.state.matches_current && <p role="alert">登录项指向其他程序或数据目录。</p>}
      {c.state?.enabled && c.state.disabled_by_system && <p role="alert">Windows 已禁用此登录项，请在任务管理器的「启动应用」中启用 AnchorDesktopCore。</p>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
      {c.notice && <p role="status">{c.notice}</p>}
      <ConfirmAction open={c.pending !== null} title={c.pending ? '启用本机核心登录自启？' : '关闭本机核心登录自启？'} busy={c.busy} onCancel={() => c.setPending(null)} onConfirm={() => void c.save()}>
        {c.pending ? '仅为当前 Windows 用户登记登录项。下次登录只启动本机后台核心，不打开窗口、不启动 Codex，也不保证早于 Codex 启动。' : '移除当前 Windows 用户的 Anchor 核心登录项。不会停止当前核心或修改已有托管授权。'}
      </ConfirmAction>
    </>}
  </section>;
}
