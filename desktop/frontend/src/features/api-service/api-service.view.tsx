import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { prototype } from '../../desktop/environment';
import { useAPIService } from './use-api-service';

export function APIServiceSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const enabled = !prototype && coreReady, c = useAPIService(enabled, onActivity);
  const disabled = c.busy || !c.data || c.needsRead;
  return <section className="detail-card" aria-labelledby="api-service-title">
    <header className="card-toolbar"><h2 id="api-service-title">API 服务</h2></header>
    {!enabled ? <p role="status">{prototype ? '桌面专用' : '未连接'}</p> : <div className="space-y-4">
      <p className="text-sm text-muted-foreground">供自定义应用调用；独立于 Codex 等托管 Agent 的本机网关。Key 可访问所有已配置的公开模型映射。</p>
      <form className="space-y-4" onSubmit={e => { e.preventDefault(); void c.save(); }}>
        <label className="model-setting-field"><span>启用 API 服务</span><Switch aria-label="启用 API 服务" checked={c.automatic} disabled={disabled} onCheckedChange={c.setAutomatic} /></label>
        <label className="model-setting-field"><span>API 端口</span><input className="input-field" type="number" min="1" max="65535" value={c.port} disabled={disabled} onChange={e => c.setPort(e.target.value)} /></label>
        <label className="model-setting-field"><span>允许局域网调用</span><Switch aria-label="允许局域网调用" checked={c.lan} disabled={disabled} onCheckedChange={c.setLAN} /></label>
        {c.lan && <p className="text-sm text-muted-foreground">通过 HTTP 接受私有 IPv4 局域网地址，所有请求仍需 Key。请在可信网络使用，并按需允许系统防火墙的此端口。</p>}
        <Button type="submit" disabled={disabled || !c.validPort}>保存监听设置</Button>
      </form>
      {c.data && <div className="space-y-2 text-sm"><p>当前监听：<code>{c.data.active_address || '未启用'}</code></p>{c.data.base_urls.map(url => <p key={url}>Base URL：<code className="select-text break-all">{url}</code></p>)}{c.data.restart_required && <p role="status">监听设置待生效，请在顶部核心菜单选择「重启核心」。重启会短暂中断请求。</p>}</div>}
      <form className="space-y-3 border-t pt-4" onSubmit={e => { e.preventDefault(); void c.issue(); }}>
        <h3 className="font-medium">签发调用 Key</h3>
        <label className="model-setting-field"><span>名称</span><input className="input-field" placeholder="例如：手机助手" value={c.label} disabled={disabled} maxLength={128} onChange={e => c.setLabel(e.target.value)} /></label>
        <label className="model-setting-field"><span>有效期（天，0 为永久）</span><input className="input-field" type="number" min="0" max="3650" value={c.days} disabled={disabled} onChange={e => c.setDays(e.target.value)} /></label>
        <Button type="submit" disabled={disabled || !c.validIssue}>签发 Key（仅显示一次）</Button>
      </form>
      {c.issued && <div className="space-y-2 rounded-md border p-3"><p role="status">已签发「{c.issued.access.label}」。请保存，下次无法查看完整 Key。</p><label className="block"><span className="text-sm">新 API Key</span><input className="input-field font-mono" readOnly autoComplete="off" spellCheck={false} value={c.issued.token} onFocus={e => e.currentTarget.select()} /></label><Button variant="outline" onClick={c.hideKey}>关闭显示</Button></div>}
      <div className="overflow-x-auto"><table className="w-full text-sm"><caption className="text-left font-medium">调用 Key</caption><thead><tr>{['名称 / ID', '有效期', '状态', '操作'].map(text => <th key={text} className="p-2 text-left">{text}</th>)}</tr></thead><tbody>{c.data?.keys.map(key => <tr key={key.id} className="border-t"><td className="p-2"><p>{key.label}</p><code className="break-all text-xs">{key.id}</code></td><td className="p-2">{key.expires_at ? new Date(key.expires_at * 1000).toLocaleString() : '永久'}</td><td className="p-2">{key.revoked ? '已撤销' : key.expires_at && key.expires_at * 1000 <= Date.now() ? '已过期' : '有效'}</td><td className="p-2"><Button variant="outline" size="sm" disabled={disabled || key.revoked} onClick={() => c.setRevokeTarget(key.id)}>撤销</Button></td></tr>)}</tbody></table>{c.data?.keys.length === 0 && <p className="py-2 text-sm text-muted-foreground">尚未签发 Key。</p>}</div>
      {c.revokeTarget && <div role="alert" className="space-y-2"><p>撤销「{c.data?.keys.find(key => key.id === c.revokeTarget)?.label}」后，使用它的应用将无法继续调用。</p><Button variant="destructive" disabled={disabled} onClick={() => void c.revoke()}>确认撤销</Button> <Button variant="outline" disabled={c.busy} onClick={() => c.setRevokeTarget(null)}>取消</Button></div>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}{c.notice && <p role="status">{c.notice}</p>}
      <Button variant="outline" disabled={c.busy} onClick={() => void c.read()}>{c.busy ? '处理中' : '重新读取'}</Button>
    </div>}
  </section>;
}
