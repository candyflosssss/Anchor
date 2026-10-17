import { useState, type FormEvent } from 'react';
import { Network, Plus, RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype } from '../../desktop/environment';
import { bindingFromValue, bindingValue, proxyAction } from './proxies.model';
import { useProxiesController } from './use-proxies-controller';
import type { ProxyProfile } from '../../desktop/contract.generated';
const field = 'h-9 min-w-0 rounded-md border border-input bg-background px-3 text-sm disabled:opacity-50';
type Controller = ReturnType<typeof useProxiesController>;
function Feedback({ c }: { c: Controller }) {
  return <>{prototype && <p role="status" className="text-sm text-muted-foreground">代理设置需要真实核心连接，预览模式不会启动 mihomo 或保存配置。</p>}{c.error && <p role="alert" className="text-sm text-destructive">{c.error}</p>}</>;
}
export function ProxyBindings({ c, providerId }: { c: Controller; providerId?: string }) {
  if (!c.data) return null;
  const data = c.data;
  const targets = data.targets.filter(t => providerId === undefined || t.provider_id === providerId);
  return <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b text-left text-muted-foreground"><th className="p-3 font-medium">供应商</th><th className="p-3 font-medium">作用范围</th><th className="p-3 font-medium">出站方式</th></tr></thead><tbody>
    {targets.map(t => <tr key={`${t.provider_id}/${t.key_id}`} className="border-b last:border-0"><td className="p-3">{t.provider_id}</td><td className="p-3">{t.key_id ? `Key · ${t.key_id}` : '供应商默认'}</td><td className="p-3"><select className={`${field} w-full max-w-xs`} aria-label={`${t.provider_id} ${t.key_id || '默认'} 代理`} value={bindingValue(data, t.provider_id, t.key_id)} disabled={c.disabled} onChange={e => {
      const input = proxyAction('bind', data.revision); input.binding = bindingFromValue(t.provider_id, t.key_id, e.target.value); void c.save(input);
    }}>{t.key_id && <option value="inherit">继承供应商</option>}<option value="direct">直连{t.key_id ? '（覆盖供应商）' : ''}</option>{data.profiles.map(p => <option key={p.id} value={`proxy:${p.id}`}>{p.name} · {p.kind}</option>)}</select></td></tr>)}
  </tbody></table>{targets.length === 0 && <p className="p-4 text-sm text-muted-foreground">添加供应商和 Key 后可在这里选择代理。</p>}</div>;
}
export function ProviderProxyPanel({ providerId, disabled }: { providerId: string; disabled: boolean }) {
  const c = useProxiesController();
  return <section className="mt-4 rounded-lg border p-4" aria-label="供应商代理设置"><div className="flex items-center justify-between gap-2"><h3 className="font-medium">出站代理</h3><Button variant="ghost" size="sm" disabled={c.busy || disabled} onClick={() => void c.refresh()}><RefreshCw />重新读取</Button></div>
    <p className="my-2 text-sm text-muted-foreground">在侧栏“代理设置”添加节点；Key 可继承、强制直连或选择其他代理。连接失败不会自动直连。</p><Feedback c={c} /><ProxyBindings c={{ ...c, disabled: c.disabled || disabled }} providerId={providerId} /></section>;
}
export function ProxiesView({ onActivity }: { onActivity?: (busy: boolean) => void }) {
  const c = useProxiesController(onActivity);
  const [editing, setEditing] = useState<ProxyProfile | 'new' | null>(null);
  const [id, setId] = useState(''); const [name, setName] = useState(''); const [kind, setKind] = useState('mihomo'); const [secret, setSecret] = useState(''); const [node, setNode] = useState('');
  const [deleting, setDeleting] = useState<ProxyProfile | null>(null);
  function edit(p: ProxyProfile | 'new') { setEditing(p); setId(p === 'new' ? '' : p.id); setName(p === 'new' ? '' : p.name); setKind(p === 'new' ? 'mihomo' : p.kind); setSecret(''); setNode(''); }
  async function submit(e: FormEvent) { e.preventDefault(); if (!c.data) return; const input = proxyAction('put', c.data.revision); input.profile = { id, name, kind, revision: 0 }; input.secret = secret || null; input.node_name = node;
    const saved = await c.save(input); setSecret(''); if (saved) setEditing(null);
  }
  return <main className="mx-auto w-full max-w-6xl space-y-6 p-4 md:p-6" aria-label="代理设置" aria-busy={c.busy}>
    <header className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="flex items-center gap-2 text-xl font-semibold"><Network className="size-5" />代理设置</h1><p className="mt-1 text-sm text-muted-foreground">只影响 Anchor 上游请求，不修改系统代理。</p></div><div className="flex gap-2"><Button variant="outline" disabled={c.busy || prototype} onClick={() => void c.refresh()}><RefreshCw />重新读取</Button><Button disabled={c.disabled} onClick={() => edit('new')}><Plus />添加代理</Button></div></header>
    <Feedback c={c} />
    {c.data && <section className="rounded-lg border p-4 text-sm"><strong>内置 mihomo {c.data.engine.version}</strong><span className="ml-3 text-muted-foreground">{c.data.engine.platform} · {c.data.engine.available ? `已内置 · ${c.data.engine.running} 个节点进程` : '当前核心未内置（外部 HTTP / SOCKS5 仍可用）'}</span><p className="mt-2 text-muted-foreground">按需启动，退出核心时回收。节点不可用会报错，不会绕过代理；远程连接使用远端主机上的代理。</p></section>}
    {editing && <form onSubmit={e => void submit(e)} className="space-y-4 rounded-lg border p-4"><h2 className="font-medium">{editing === 'new' ? '添加代理' : '编辑代理'}</h2><div className="grid gap-4 sm:grid-cols-3"><label className="space-y-1 text-sm">ID<Input value={id} required disabled={editing !== 'new' || c.disabled} onChange={e => setId(e.target.value)} /></label><label className="space-y-1 text-sm">名称<Input value={name} required maxLength={100} disabled={c.disabled} onChange={e => setName(e.target.value)} /></label><label className="space-y-1 text-sm">类型<select className={`${field} block w-full`} value={kind} disabled={c.disabled} onChange={e => { setKind(e.target.value); setSecret(''); }}><option value="mihomo">mihomo 节点（Clash YAML）</option><option value="http">HTTP / HTTPS 代理</option><option value="socks5">SOCKS5 代理</option></select></label></div>
      <label className="block space-y-1 text-sm">{kind === 'mihomo' ? '节点 YAML（多节点配置需填写节点名称）' : '代理 URL（包含端口，可包含用户名和密码）'}<textarea className="block min-h-32 w-full rounded-md border bg-background p-3 font-mono text-sm" aria-label="代理秘密配置" value={secret} required={editing === 'new'} autoComplete="off" spellCheck={false} disabled={c.disabled} onChange={e => setSecret(e.target.value)} /></label>
      {kind === 'mihomo' && <label className="block max-w-md space-y-1 text-sm">节点名称（单节点可留空）<Input value={node} disabled={c.disabled} onChange={e => setNode(e.target.value)} /></label>}
      <p className="text-xs text-muted-foreground">秘密不会回显，编辑留空表示保留；保存在核心的受限状态目录中，非静态加密。仅导入节点，不执行配置中的规则、TUN 或控制器。暂不支持订阅 URL 自动刷新。</p>
      <div className="flex justify-end gap-2"><Button type="button" variant="outline" disabled={c.busy} onClick={() => { setEditing(null); setSecret(''); }}>取消</Button><Button disabled={c.disabled}>保存</Button></div></form>}
    <section className="rounded-lg border"><h2 className="border-b p-4 font-medium">已配置代理</h2>{!c.data ? <p className="p-4 text-sm text-muted-foreground">{c.busy ? '正在读取…' : '暂无可显示的配置'}</p> : c.data.profiles.length === 0 ? <p className="p-6 text-sm text-muted-foreground">尚未添加代理。默认全部直连。</p> : <ul className="divide-y">{c.data.profiles.map(p => <li key={p.id} className="flex flex-wrap items-center justify-between gap-3 p-4"><div><span className="font-medium">{p.name}</span><span className="ml-3 text-sm text-muted-foreground">{p.id} · {p.kind}</span></div><div className="flex gap-2"><Button variant="outline" size="sm" disabled={c.disabled} onClick={() => edit(p)}>编辑</Button><Button variant="ghost" size="sm" disabled={c.disabled} onClick={() => setDeleting(p)}>删除</Button></div></li>)}</ul>}</section>
    <section className="rounded-lg border"><h2 className="border-b p-4 font-medium">供应商与 Key</h2><ProxyBindings c={c} /></section>
    <ConfirmAction open={deleting !== null} title="删除代理？" busy={c.busy} onCancel={() => setDeleting(null)} onConfirm={() => { if (!deleting || !c.data) return; const input = proxyAction('delete', c.data.revision); input.profile = deleting; void c.save(input).then(ok => { if (ok) setDeleting(null); }); }}>删除“{deleting?.name}”。仍有引用时核心会拒绝删除。</ConfirmAction>
  </main>;
}
