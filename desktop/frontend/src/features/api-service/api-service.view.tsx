import { useRef } from 'react';
import { Check, Copy, KeyRound, Monitor, Plus, RefreshCw, Server, Wifi } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype } from '../../desktop/environment';
import { useAPIService } from './use-api-service';
import './api-service.css';

export function APIServiceSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const createButton = useRef<HTMLButtonElement>(null);
  const enabled = !prototype && coreReady, c = useAPIService(enabled, onActivity);
  const disabled = c.busy || !c.data || c.needsRead;
  const running = !!c.data?.active_address;
  const revokeLabel = c.data?.keys.find(key => key.id === c.revokeTarget)?.label;
  return <section className="detail-card api-service" aria-labelledby="api-service-title" aria-busy={c.busy}>
    <header className="api-service-header">
      <div className="api-service-heading"><span className="api-service-icon"><Server size={18} /></span><h2 id="api-service-title">API 服务</h2>
        <span className={`api-status ${running ? 'api-status-live' : ''}`}>{!enabled ? prototype ? '桌面专用' : '未连接' : !c.data ? c.busy ? '读取中' : '状态未知' : running ? '运行中' : '未启用'}</span>
      </div>
      {enabled && <Button variant="ghost" size="sm" disabled={c.busy} onClick={() => void c.read()}><RefreshCw className={c.busy ? 'loading-icon' : ''} />重新读取</Button>}
    </header>
    {enabled && <>
      {c.data?.restart_required && <div role="status" className="api-restart"><RefreshCw size={16} /><div><strong>设置已保存，待重启生效</strong><p>在顶部设备菜单选择「重启核心」，请求会短暂中断。</p></div></div>}
      <div className="api-service-grid">
        <form className="api-settings" onSubmit={e => { e.preventDefault(); void c.save(); }}>
          <h3>服务设置</h3>
          <label className="api-setting-row"><span>启用 API 服务</span><Switch aria-label="启用 API 服务" checked={c.automatic} disabled={disabled} onCheckedChange={c.setAutomatic} /></label>
          <label className="api-setting-row"><span>端口</span><Input aria-label="API 端口" className="api-port" type="number" min="1" max="65535" value={c.port} disabled={disabled} aria-invalid={!c.validPort} onChange={e => c.setPort(e.target.value)} /></label>
          {!c.validPort && <p className="field-error">端口需为 1–65535 的整数。</p>}
          <label className="api-setting-row"><span className="api-inline-label"><Wifi size={15} />允许局域网调用</span><Switch aria-label="允许局域网调用" checked={c.lan} disabled={disabled} onCheckedChange={c.setLAN} /></label>
          {c.lan && <p className="api-network-note">仅可信局域网 · HTTP · 需应用 Key</p>}
          <div className="api-settings-footer"><span role="status">{c.dirty ? '有未保存的更改' : ''}</span><Button variant="outline" size="sm" type="submit" disabled={disabled || !c.validPort || !c.dirty}>保存设置</Button></div>
        </form>
        <div className="api-addresses">
          <div className="api-section-heading"><h3>接入地址</h3><span>Base URL</span></div>
          {c.data ? c.data.base_urls.length ? <div className="api-address-list">{c.data.base_urls.map(url => {
            const local = url.startsWith('http://127.0.0.1:');
            const Icon = local ? Monitor : Wifi;
            return <div className="api-address" key={url}><span className="api-address-label"><Icon size={14} />{local ? '核心所在设备' : '局域网'}</span><div className="api-address-value"><code tabIndex={0}>{url}</code><Button variant="ghost" size="icon" aria-label={`复制接入地址 ${url}`} onClick={() => void c.copy(url)}><Copy /></Button></div></div>;
          })}</div> : <div className="api-address-empty"><Server size={24} /><strong>暂无可用地址</strong><span>{c.data.restart_required ? '重启核心后更新接入地址' : '启用并保存设置，重启核心后可用'}</span></div> : <p className="api-address-empty">{c.busy ? '正在读取服务状态…' : '未能读取接入地址'}</p>}
          {!c.issueOpen && c.copyNotice && <p className="api-copy-notice" role="status">{c.copyNotice}</p>}
        </div>
      </div>
      <div className="api-keys">
        <header className="api-keys-header"><div className="api-inline-label"><h3>应用 Key</h3>{c.data && <span className="tab-count">{c.data.keys.length}</span>}</div><Button ref={createButton} size="sm" disabled={disabled} onClick={c.openIssue}><Plus />创建 Key</Button></header>
        {c.data?.keys.length === 0 && <div className="api-keys-empty"><KeyRound size={20} /><span>还没有应用 Key</span></div>}
        {!!c.data?.keys.length && <ul className="api-key-list">{c.data.keys.map(key => {
          const state = key.revoked ? '已撤销' : key.expires_at && key.expires_at * 1000 <= Date.now() ? '已过期' : '有效';
          return <li className="api-key-row" key={key.id}>
            <span className="api-key-icon"><KeyRound size={16} /></span>
            <div className="api-key-name"><strong>{key.label}</strong><code>ID · {key.id}</code></div>
            <span className={`api-status ${state === '有效' ? 'api-status-live' : ''}`}>{state}</span>
            <span className="api-key-expiry">{key.expires_at ? `${new Date(key.expires_at * 1000).toLocaleDateString()} 到期` : '永久有效'}</span>
            <Button variant="ghost" size="sm" disabled={disabled || key.revoked} aria-label={`撤销 ${key.label}`} onClick={() => c.setRevokeTarget(key.id)}>撤销</Button>
          </li>;
        })}</ul>}
      </div>
      {!c.issueOpen && c.error && <p role="alert" className="api-feedback error-feedback">{c.error}</p>}
      {c.needsRead && !c.issueOpen && <p role="status" className="api-feedback">请重新读取后再操作；读取无法恢复未收到的完整 Key。</p>}
      {c.notice && c.notice !== '设置已保存。' && <p role="status" className="api-feedback">{c.notice}</p>}
    </>}
    <Dialog open={c.issueOpen} onOpenChange={open => { if (!open) c.closeIssue(); }}>
      <DialogContent className="api-key-dialog" closeLabel="关闭创建 Key" onCloseAutoFocus={e => { e.preventDefault(); createButton.current?.focus(); }} onRequestClose={c.closeIssue} onEscapeKeyDown={e => { if (c.busy) e.preventDefault(); }} onInteractOutside={e => { if (c.busy) e.preventDefault(); }}>
        <div className="api-dialog-heading"><span className="api-service-icon">{c.issued ? <Check size={20} /> : <KeyRound size={20} />}</span><DialogTitle>{c.issued ? 'Key 已创建' : '创建应用 Key'}</DialogTitle><DialogDescription>{c.issued ? `「${c.issued.access.label}」的完整 Key 仅显示这一次，请保存。` : '此 Key 可调用所有已配置的模型，名称会显示在调用日志中。'}</DialogDescription></div>
        {c.issued ? <div className="api-issued">
          <label htmlFor="api-new-key">API Key</label><Input id="api-new-key" autoFocus readOnly autoComplete="off" spellCheck={false} value={c.issued.token} onFocus={e => e.currentTarget.select()} />
          {c.copyNotice && <p role="status" className="api-copy-notice">{c.copyNotice}</p>}
          <div className="api-dialog-footer"><Button variant="outline" onClick={() => void c.copy(c.issued!.token)}><Copy />复制 Key</Button><Button onClick={c.closeIssue}>完成</Button></div>
        </div> : <form onSubmit={e => { e.preventDefault(); void c.issue(); }}>
          <div className="api-dialog-fields"><label htmlFor="api-key-label">应用名称<Input id="api-key-label" placeholder="例如：手机助手" value={c.label} disabled={disabled} maxLength={128} onChange={e => c.setLabel(e.target.value)} /></label>
            <label htmlFor="api-key-days">有效期（天）<Input id="api-key-days" type="number" min="0" max="3650" value={c.days} disabled={disabled} onChange={e => c.setDays(e.target.value)} /><span className="field-hint">填 0 表示永久有效</span></label>
            {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
            {c.needsRead && <p role="status" className="field-hint">请关闭弹窗后重新读取；完整 Key 无法再次查看。</p>}
          </div><div className="api-dialog-footer"><Button variant="outline" disabled={c.busy} onClick={c.closeIssue}>取消</Button><Button type="submit" disabled={disabled || !c.validIssue}>{c.busy ? '正在创建…' : '创建 Key'}</Button></div>
        </form>}
      </DialogContent>
    </Dialog>
    <ConfirmAction open={!!c.revokeTarget} title={`撤销「${revokeLabel ?? ''}」？`} busy={c.busy} onCancel={() => c.setRevokeTarget(null)} onConfirm={() => void c.revoke()}>使用此 Key 的应用将无法继续调用。撤销后无法恢复。</ConfirmAction>
  </section>;
}
