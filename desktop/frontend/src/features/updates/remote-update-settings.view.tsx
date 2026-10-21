import type { SessionInfo } from '../../desktop/contract.generated';
import { Button } from '../../components/ui/button';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { useRemoteUpdateController } from './use-remote-update-controller';

export function RemoteUpdateSettings({ session, onActivity }: { session: SessionInfo; onActivity: (busy: boolean) => void }) {
  const c = useRemoteUpdateController(session.instance_id, onActivity);
  const currentVersion = c.info?.phase === 'applied' ? c.info.current_version : session.version || c.info?.current_version;
  return <section className="space-y-4" aria-labelledby="remote-updates-title">
    <h2 id="remote-updates-title" className="text-base font-semibold">SSH 被控机更新 · {session.target}</h2>
    <div className="space-y-4 rounded-lg border p-5">
      <dl className="grid gap-3 text-sm sm:grid-cols-[8rem_1fr]"><dt className="text-muted-foreground">被控机当前版本</dt><dd className="break-all">{currentVersion || '未提供'}</dd></dl>
      <Button variant="outline" disabled={c.busy} onClick={() => void c.check()}>{c.operation === 'checking' ? '检查中…' : '检查被控机更新'}</Button>
      {c.info && <p role="status" className="text-sm">{c.info.phase === 'applied' ? `已更新到 ${c.info.current_version}，服务已恢复` : c.info.available ? `${c.info.current_version} → ${c.info.latest_version}` : '没有可用更新'}</p>}
      {c.info?.available && <Button disabled={c.busy} onClick={() => c.setConfirm(true)}>{c.busy ? '更新中…' : '更新这台被控机…'}</Button>}
      {c.error && <p role="alert" className="text-sm text-destructive">{c.error}</p>}
    </div>
    <ConfirmAction open={c.confirm} title={`更新被控机 ${session.target}？`} busy={c.busy} onCancel={() => c.setConfirm(false)} onConfirm={() => void c.install()}>
      <p>将在这台被控机下载并校验 {c.info?.latest_version}，准备好后短暂切换服务一次。配置和连接保留，本机服务继续运行。</p>
      <p>切换时正在生成的回答可能中断。更新完成后会检查服务是否恢复；结果不明时请先检查状态。</p>
    </ConfirmAction>
  </section>;
}
