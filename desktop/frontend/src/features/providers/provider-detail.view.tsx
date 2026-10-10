import { CircleHelp, KeyRound, Pencil, ShieldCheck, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { StatusBadge } from '../../components/shared/status-badge';
import type { Provider } from './provider.model';
export function ProviderDetail({ provider: p, blocked, onClose, onEdit, onToggle }: { provider: Provider; blocked: boolean; onClose: () => void; onEdit: () => void; onToggle: () => void }) {
  return <aside className="provider-detail" aria-label={`${p.id} 的详情`}>
    <header><span className="eyebrow">供应商详情</span><Button variant="ghost" size="icon" onClick={onClose} aria-label="收起供应商详情"><X /></Button></header>
    <div className="detail-identity"><span className="provider-avatar large">{Array.from(p.id)[0]}</span><h2>{p.id}</h2><StatusBadge enabled={p.enabled} /></div>
    <dl className="detail-properties">
      <dt>API 根地址</dt><dd className="mono break-all">{p.base_url}</dd>
      <dt>协议</dt><dd>Responses</dd>
      <dt>凭据</dt><dd className="flex items-center gap-2"><KeyRound size={14} />{p.has_secret ? '已配置 · 密钥不可回显' : '未配置密钥'}</dd>
      <dt>请求超时</dt><dd>{p.timeout_seconds} 秒</dd>
      <dt>最大并发</dt><dd>{p.max_concurrent} 个请求</dd>
      <dt>配置版本</dt><dd className="mono">revision {p.revision}</dd>
    </dl>
    <div className="detail-note"><CircleHelp size={16} /><div><strong>上游连通性未测试</strong><p>启用状态仅表示配置允许参与路由，当前版本没有供应商测试入口。</p></div></div>
    <div className="detail-actions"><Button variant="outline" disabled={blocked} onClick={onEdit}><Pencil />编辑配置</Button><Button variant="ghost" disabled={blocked} onClick={onToggle}>{p.enabled ? '停用供应商' : '启用供应商'}</Button></div>
    <p className="detail-footnote"><ShieldCheck size={14} />停用不会删除配置或已存密钥。</p>
  </aside>;
}
