import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import { protocolName } from './provider.model';
import { importReason } from './ccswitch-import.model';
import type { CCImportController } from './use-ccswitch-import';

export function CCImportDialog({ c }: { c: CCImportController }) {
  if (!c.open) return null;
  const r = c.report;
  return <Dialog open onOpenChange={open => { if (!open) c.close(); }}>
    <DialogContent className="action-dialog" closeLabel="关闭 CC Switch 导入" onRequestClose={c.close}
      onInteractOutside={e => { e.preventDefault(); }} onEscapeKeyDown={e => { e.preventDefault(); c.close(); }}>
      <header className="editor-header"><DialogTitle className="text-xl font-semibold">从 CC Switch 导入</DialogTitle>
        <DialogDescription>读取当前实例所在设备的全部平台和供应商，按 Anchor 的供应商、Key 和模型映射启用。</DialogDescription></header>
      <div className="editor-fields space-y-4">
        <div className="field"><label htmlFor="ccs-path">CC Switch 数据库路径</label><Input id="ccs-path" disabled={c.busy} value={c.path} placeholder="默认 ~/.cc-switch/cc-switch.db" onChange={e => c.changePath(e.target.value)} />
          <p className="text-sm text-muted-foreground">留空自动检测；远程实例读取远程账户路径。原数据库保持只读。</p></div>
        {c.error && <p role="alert" className="text-destructive">{c.error}</p>}
        {c.busy && <p role="status">{r ? '正在导入…' : '正在读取全部供应商…'}</p>}
        {r && !r.failure && <>
          <p className="text-sm break-all">来源：{r.database_path}</p>
          <p role="status">{r.applied ? `已启用 ${r.imported_providers} 个供应商、${r.imported_keys} 个 Key，更新 ${r.imported_models} 个模型。`
            : `将启用 ${c.counts!.providers} 个供应商、${c.counts!.keys} 个 Key，添加 ${c.counts!.mappings} 个模型来源。`}</p>
          <div className="max-h-72 overflow-auto space-y-3" aria-label="导入预览">
            {r.providers.map(p => <section key={p.id} className="rounded-md border p-3 text-sm"><div className="font-medium break-all">{p.id} · {p.status === 'exists' ? '已存在，保留现有配置' : r.applied ? '已导入' : '待导入'}</div>
              <p className="text-muted-foreground break-all">{p.sources.map(s => `${s.platform} / ${s.name}`).join('、')}</p>
              <p className="break-all">{p.base_urls.join('、')}</p><p>{p.keys.map(k => protocolName[k.protocol] ?? k.protocol).join('、')} · {p.keys.length} 个 Key</p>
              {r.models.filter(m => m.provider_id === p.id).map(m => <p key={m.id + m.upstream} className="break-all">{m.id} → {m.upstream}（{m.key_ids.length} 个来源 Key）</p>)}
            </section>)}
            {r.issues.map((issue, i) => <p key={i} className="text-sm break-all">{issue.platform} / {issue.name}：{importReason[issue.reason] ?? '配置未转换'}</p>)}
            {!r.providers.length && !r.issues.length && <p>CC Switch 中没有供应商。</p>}
          </div>
          <p className="text-sm text-muted-foreground">保存后尚未验证上游连通性。已有供应商不会覆盖；客户端托管沿用 Anchor 的 Agent 管理流程。</p>
        </>}
      </div>
      <footer className="editor-footer"><Button variant="outline" disabled={c.busy} onClick={c.close}>关闭</Button>
        <Button variant="outline" disabled={c.busy} onClick={() => void c.preview()}>读取并预览</Button>
        <Button disabled={c.busy || !r || !!r.failure || r.applied || !c.counts?.providers} onClick={() => void c.apply()}>确认导入并启用</Button></footer>
    </DialogContent>
  </Dialog>;
}
