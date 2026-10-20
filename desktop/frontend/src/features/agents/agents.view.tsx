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
        const canRebuild = target?.supported && target.agent === 'codex' && ['ready', 'config_unsupported'].includes(target.status);
        const canToggle = !!binding || canConfigure || canRebuild;
        const panelId = `agent-panel-${row.id}`;
        return <article className="agent-platform" key={row.id}>
          <div className="agent-platform-header">
            <button type="button" className="agent-platform-heading" aria-label={`${name} 托管设置`} aria-expanded={open} aria-controls={panelId}
              disabled={c.busy} onClick={() => open ? c.cancel() : c.select(row)}>
              <span className="agent-brand">{brand ? <img src={brand.logo} alt="" className={brand.monochrome ? 'agent-brand-monochrome' : undefined} /> : <Bot size={28} />}</span>
              <span className="agent-platform-name">{name}</span>
              {binding?.enabled && binding.status !== 'applied' && <span className="agent-platform-status" aria-label={`${name} 托管状态`}>{managementStatus[binding.status] ?? binding.status}</span>}
              <ChevronDown size={16} className="agent-chevron" />
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
            {(canConfigure || canRebuild) && <form onSubmit={e => { e.preventDefault(); c.update(); }}>
              <fieldset disabled={c.busy}>
                <div className="field"><label htmlFor={`agent-default-${row.id}`}>默认模型</label>
                  <select id={`agent-default-${row.id}`} className="select-field w-full" value={c.model} onChange={e => c.setModel(e.target.value)}>
                    <option value="">请选择</option>{c.data!.models.map(m => { const available = c.availableModels.some(v => v.id === m.id); return <option key={m.id} value={m.id} disabled={!available}>{m.id}{available ? '' : '（无启用的供应商）'}</option>; })}
                  </select>
                </div>
                {!c.availableModels.length && <p className="field-error">现有模型映射没有启用的候选供应商。请在模型映射中检查供应商状态。</p>}
              </fieldset>
              <div className="agent-settings-actions">
                {canConfigure && <Button type="submit" disabled={c.busy || !c.availableModels.some(m => m.id === c.model)}>{binding ? '更新配置' : '开启托管'}</Button>}
                {canRebuild && <Button type="button" variant="outline" disabled={c.busy || !c.availableModels.some(m => m.id === c.model)} onClick={() => c.update(true)}>备份并重建配置</Button>}
              </div>
            </form>}
          </div>}
        </article>;
      })}</section>}
    </>}
    <ConfirmAction open={!!c.updateInput} title={c.updateInput?.rebuild ? '备份并重建配置？' : '更新配置？'} busy={c.busy} onCancel={() => c.setUpdateInput(null)} onConfirm={() => { void c.confirmUpdate(); }}>
      <p className="break-all">{c.selected?.config_root}</p><p>默认模型：{c.updateInput?.model}</p>{c.updateInput?.rebuild
        ? <p>将先备份整个 config.toml，再替换为干净的托管配置，并同步最新模型列表。原来的其他设置也会从当前配置移除，完整原件和旧恢复记录保留在 Anchor 数据目录的 recovery 中。停用托管只清除新托管内容，不会自动恢复坏配置。完成后请重启 Codex。</p>
        : <p>将使用当前选择更新连接配置，并同步最新模型列表。其他软件改过的托管字段也会重新应用，外部值会先保存，停用托管时恢复；其他设置不动。更新后请重启对应 Agent。</p>}
    </ConfirmAction>
    <ConfirmAction open={!!c.disableTarget} title="停用此 Agent 的托管？" busy={c.busy} onCancel={() => c.setDisableTarget(null)} onConfirm={() => { if (c.disableTarget) void c.manage(c.disableTarget, 'disable'); }}>
      <p className="break-all">{c.disableTarget?.config_root}</p><p>按字段恢复 Anchor 写入的配置并停用网关凭据。第三方修改会保留并报告冲突。完成后请重启对应 Agent（DSH Desktop 关闭窗口可能只是隐藏，请完全退出）；DSH 请新建会话。</p>
    </ConfirmAction>
  </div>;
}
