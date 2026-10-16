import { Download, RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype } from '../../desktop/environment';
import { channels, type Channel } from './update.model';
import { useUpdateController } from './use-update-controller';

export function UpdateSettings({ onActivity }: { onActivity: (busy: boolean) => void }) {
  const c = useUpdateController(!prototype, onActivity);
  const busy = c.busy;
  return <section className="space-y-4" aria-labelledby="updates-title">
    <h2 id="updates-title" className="text-base font-semibold">在线更新</h2>
    {prototype ? <p role="status" className="rounded-lg border p-4 text-sm text-muted-foreground">在线更新仅在桌面模式提供。</p> : <>
      <div className="space-y-4 rounded-lg border p-5">
        <dl className="grid gap-3 text-sm sm:grid-cols-[6rem_1fr]">
          <dt className="text-muted-foreground">当前版本</dt><dd>{c.info?.current_version ?? '读取中…'}</dd>
          <dt className="text-muted-foreground">更新源</dt><dd className="break-all">{c.info?.source ?? '由本机 CLI 配置'}</dd>
        </dl>
        <div className="flex flex-wrap items-center gap-3">
          <label htmlFor="update-channel" className="text-sm">更新渠道</label>
          <select id="update-channel" className="h-9 rounded-md border bg-background px-3 text-sm" disabled={busy} value={c.channel} onChange={e => c.setChannel(e.target.value as Channel)}>
            {channels.map(value => <option key={value} value={value}>{value === 'dev' ? 'dev · 快速测试' : value}</option>)}
          </select>
          <Button variant="outline" disabled={busy} onClick={() => void c.check()}><RefreshCw size={16} />{c.operation === 'checking' ? '检查中…' : '检查更新'}</Button>
        </div>
      </div>
      {c.info && c.info.phase !== 'idle' && <div className="space-y-4 rounded-lg border p-5" aria-live="polite">
        <h2 className="font-medium">{c.info.phase === 'installing' ? '正在交接安装器' : c.info.phase === 'downloaded' ? '安装包已下载并通过 SHA256 校验' : c.info.available ? `发现新版本 ${c.info.latest_version}` : c.info.latest_version ? '当前版本无需升级' : '当前渠道尚无已发布的兼容桌面安装包'}</h2>
        {c.info.latest_version && <p className="text-sm text-muted-foreground">候选版本：{c.info.latest_version} · {c.info.channel} · {c.info.install_kind === 'deb' ? 'Linux .deb' : 'Windows 安装器'}</p>}
        {c.info.download_path && <p className="break-all text-xs text-muted-foreground">已校验文件：{c.info.download_path}</p>}
        {c.info.available && c.info.phase === 'checked' && <Button disabled={busy} onClick={() => void c.download()}><Download size={16} />{c.operation === 'downloading' ? '下载并校验中…' : '下载更新'}</Button>}
        {c.info.phase === 'downloaded' && <Button disabled={busy} onClick={() => c.setConfirm(true)}>安装更新…</Button>}
      </div>}
      {c.error && <p role="alert" className="rounded-lg border border-destructive/30 p-4 text-sm text-destructive">{c.error}</p>}
      <ConfirmAction open={c.confirm} title={`安装 ${c.info?.latest_version ?? ''}？`} busy={busy} onCancel={() => c.setConfirm(false)} onConfirm={() => void c.install()}>
        <p>确认后将正常停止本机核心，等待退出并关闭此窗口，然后打开已校验的安装包。不会强杀进程或自动注册服务。</p>
        <p>请结束正在使用代理的请求并在其他 Anchor 窗口点击「退出界面」。安装完成后重新打开应用；数据目录保留。Linux 需在系统安装器中授权。</p>
      </ConfirmAction>
    </>}
  </section>;
}
