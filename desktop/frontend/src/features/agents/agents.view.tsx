import { Bot, RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype } from '../../desktop/environment';
import { managementStatus, targetStatus } from './agent.model';
import { useAgentController } from './use-agent-controller';

export function AgentsView({ onActivity }: { onActivity: (busy: boolean) => void }) {
  const c = useAgentController(!prototype, onActivity);
  return <div className="workspace">
    <header className="page-header"><div><div className="eyebrow">本机接入</div><h1>Agent 托管</h1></div>
      <Button variant="outline" disabled={prototype || c.busy} onClick={() => void c.read()}><RefreshCw size={16} />重新识别</Button></header>
    {prototype ? <div className="empty-state"><Bot size={32} /><h2>本机发现仅在桌面模式提供</h2></div> : <>
      {c.error && <p role="alert" className="feedback error-feedback">{c.error}</p>}
      {c.notice && <p role="status" className="feedback notice-feedback">{c.notice}</p>}
      {c.loading && <p role="status">正在读取本机 Agent 与托管状态…</p>}
      {c.data && <>
        <section className="detail-card"><h2>发现的 Agent</h2>
          {!c.data.targets.length && <p className="card-hint">未发现配置目录。</p>}
          <div className="divide-y">{c.data.targets.map(t => <article key={t.id} className="flex flex-wrap items-center justify-between gap-3 py-4">
            <div className="min-w-0"><h3 className="font-semibold">{t.agent === 'codex' ? 'Codex' : t.agent}</h3><p className="break-all font-mono text-sm">{t.config_root}</p>
              <p className="card-hint">{targetStatus[t.status] ?? t.status}{t.catalog_version && ` · 本地缓存版本 ${t.catalog_version}`}</p></div>
            <Button variant="outline" disabled={c.busy || !t.supported || t.status !== 'ready'} onClick={() => c.select(t)}>设置托管</Button>
          </article>)}</div>
        </section>
        {c.selected && <section className="detail-card"><h2>{c.binding ? '更新 Codex 托管' : '启用 Codex 托管'}</h2>
          <p className="card-hint break-all">目标：{c.selected.config_root}</p>
          <form onSubmit={e => { e.preventDefault(); void c.enroll(); }}>
            <fieldset disabled={c.busy} className="editor-fields">
              <div className="field"><label htmlFor="agent-default">默认模型</label><select id="agent-default" className="select-field w-full" value={c.model} onChange={e => { c.setModel(e.target.value); c.setApproved(false); }}>
                <option value="">请选择</option>{c.availableModels.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}</select></div>
              {!c.availableModels.length && <p className="field-error">现有模型映射中没有可供此 Codex 使用的模型。请在模型映射中核对工具、流式能力、启用的供应商与 Codex 模型信息。</p>}
              <div className="field"><h3 className="font-semibold">使用已有模型映射</h3>
                <p className="card-hint">以下模型将自动提供给 Codex；上游路由沿用模型映射，无需在这里重复配置。</p>
                <ul className="mt-2 flex flex-wrap gap-2" aria-label="将提供给 Codex 的模型">{c.availableModels.map(m => <li className="rounded-md border px-2 py-1 text-sm" key={m.id}>{m.id}</li>)}</ul>
                {!!c.unavailableModels.length && <p className="card-hint mt-2">暂不可用：{c.unavailableModels.map(m => m.id).join('、')}。请在模型映射中核对工具、流式能力、启用的供应商与当前 Codex 版本的模型信息。</p>}
              </div>
              <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" checked={c.approved} onChange={e => c.setApproved(e.target.checked)} />
                <span>允许 Anchor 管理此目录的连接与默认模型，并按上方已有映射生成 Codex 模型列表。</span></label>
            </fieldset>
            <div className="mt-4 flex flex-wrap gap-2"><Button type="submit" disabled={c.busy || !c.approved || !c.availableModels.some(m => m.id === c.model)}>保存并应用托管</Button><Button type="button" variant="outline" disabled={c.busy} onClick={c.cancel}>取消</Button></div>
          </form>
        </section>}
        <section className="detail-card"><h2>已登记的托管目标</h2>{!c.data.bindings.length && <p className="card-hint">还没有托管目标。</p>}
          <div className="divide-y">{c.data.bindings.map(b => <article key={b.id} className="py-4">
            <div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-semibold">{b.id}</h3><span className="tag">{b.enabled ? managementStatus[b.status] ?? b.status : '已停用'}</span></div>
            <p className="mt-2 break-all font-mono text-sm">{b.config_root}</p><p className="card-hint">默认：{b.model} · 元数据版本：{b.catalog_version || '无'} · revision {b.revision}</p>
            <p className="mt-2 break-all text-sm">授权的模型：{b.available_models.join('、') || '暂无兼容模型'}</p>
            <div className="mt-3 flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={c.busy || !b.enabled} onClick={() => void c.manage(b, 'sync')}>同步模型目录</Button>
              <Button size="sm" variant="outline" disabled={c.busy} onClick={() => b.enabled ? c.setDisableTarget(b) : void c.manage(b, 'enable')}>{b.enabled ? '停用并恢复配置' : '启用并应用'}</Button></div>
          </article>)}</div>
        </section>
      </>}
    </>}
    <ConfirmAction open={!!c.disableTarget} title="停用此 Agent 的托管？" busy={c.busy} onCancel={() => c.setDisableTarget(null)} onConfirm={() => { if (c.disableTarget) void c.manage(c.disableTarget, 'disable'); }}>
      <p className="break-all">{c.disableTarget?.config_root}</p><p>按字段恢复 Anchor 写入的配置并停用网关凭据。第三方修改会保留并报告冲突。完成后请重启 Codex。</p>
    </ConfirmAction>
  </div>;
}
