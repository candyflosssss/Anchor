import { Bot, ChevronDown, RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype } from '../../desktop/environment';
import { managementStatus, targetStatus } from './agent.model';
import { agentBrands } from './agent-brands';
import { useAgentController } from './use-agent-controller';
import './agents.css';

export function AgentsView({ onActivity }: { onActivity: (busy: boolean) => void }) {
  const c = useAgentController(!prototype, onActivity);
  return <div className="workspace agent-workspace">
    <header className="page-header"><h1>Agent 托管</h1>
      <Button variant="outline" disabled={prototype || c.busy} onClick={() => void c.read()}><RefreshCw size={16} />重新识别</Button></header>
    {prototype ? <div className="empty-state"><Bot size={32} /><h2>本机发现仅在桌面模式提供</h2></div> : <>
      {c.error && <p role="alert" className="feedback error-feedback">{c.error}</p>}
      {c.notice && <p role="status" className="feedback notice-feedback">{c.notice}</p>}
      {c.loading && <p role="status">正在读取本机 Agent 与托管状态…</p>}
      {c.data && <section className="agent-list" aria-label="Agent 平台">{c.rows.map(row => {
        const { target, binding } = row;
        const brand = agentBrands[row.agent];
        const name = brand?.name ?? row.agent;
        const open = c.expanded?.id === row.id;
        const canConfigure = target?.supported && target.status === 'ready';
        const canToggle = !!binding || canConfigure;
        const panelId = `agent-panel-${row.id}`;
        return <article className="agent-platform" key={row.id}>
          <div className="agent-platform-header">
            <button type="button" className="agent-platform-heading" aria-label={`${name} 托管设置`} aria-expanded={open} aria-controls={panelId}
              disabled={c.busy} onClick={() => open ? c.cancel() : c.select(row)}>
              <span className="agent-brand">{brand ? <img src={brand.logo} alt="" className={brand.monochrome ? 'agent-brand-monochrome' : undefined} /> : <Bot size={28} />}</span>
              <span className="agent-platform-name">{name}</span><ChevronDown size={16} className="agent-chevron" />
            </button>
            <Switch aria-label={`${name} 托管`} checked={!!binding?.enabled} disabled={c.busy || !canToggle}
              onCheckedChange={checked => {
                if (binding) { if (checked) void c.manage(binding, 'enable'); else c.setDisableTarget(binding); }
                else c.select(row);
              }} />
          </div>
          {open && <div id={panelId} className="agent-settings" role="region" aria-label={`${name} 托管设置`}>
            <dl className="agent-details">
              <div><dt>配置目录</dt><dd className="font-mono">{target?.config_root ?? binding?.config_root ?? '未发现配置目录'}</dd></div>
              {(target || binding) && <div><dt>状态</dt><dd>{binding ? (binding.enabled ? managementStatus[binding.status] ?? binding.status : '已停用') : targetStatus[target!.status] ?? target!.status}</dd></div>}
              {(target?.catalog_version || binding?.catalog_version) && <div><dt>缓存版本</dt><dd>{target?.catalog_version || binding?.catalog_version}</dd></div>}
              {!canConfigure && binding && <div><dt>默认模型</dt><dd>{binding.model}</dd></div>}
            </dl>
            {!target && !binding && <p className="card-hint">未发现此平台的本机配置。</p>}
            {canConfigure && <form onSubmit={e => { e.preventDefault(); void c.enroll(); }}>
              <fieldset disabled={c.busy}>
                <div className="field"><label htmlFor={`agent-default-${row.id}`}>默认模型</label>
                  <select id={`agent-default-${row.id}`} className="select-field w-full" value={c.model} onChange={e => c.setModel(e.target.value)}>
                    <option value="">请选择</option>{c.availableModels.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}
                  </select>
                </div>
                {!c.availableModels.length && <p className="field-error">现有模型映射中没有可供此 Codex 使用的模型。请在模型映射中核对工具、流式能力、启用的供应商与 Codex 模型信息。</p>}
              </fieldset>
              <div className="agent-settings-actions"><Button type="submit" disabled={c.busy || !c.availableModels.some(m => m.id === c.model)}>{binding ? '保存并应用' : '开启托管'}</Button></div>
            </form>}
            {binding && <div className="agent-settings-actions"><Button size="sm" variant="outline" disabled={c.busy || !binding.enabled} onClick={() => void c.manage(binding, 'sync')}>同步模型目录</Button></div>}
          </div>}
        </article>;
      })}</section>}
    </>}
    <ConfirmAction open={!!c.disableTarget} title="停用此 Agent 的托管？" busy={c.busy} onCancel={() => c.setDisableTarget(null)} onConfirm={() => { if (c.disableTarget) void c.manage(c.disableTarget, 'disable'); }}>
      <p className="break-all">{c.disableTarget?.config_root}</p><p>按字段恢复 Anchor 写入的配置并停用网关凭据。第三方修改会保留并报告冲突。完成后请重启 Codex。</p>
    </ConfirmAction>
  </div>;
}
