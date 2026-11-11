import type { CodexSettingsState } from '../../desktop/contract.generated';
import { Switch } from '../../components/ui/switch';
import { Button } from '../../components/ui/button';
import { prototype } from '../../desktop/environment';
import { useCodexSettings } from './use-codex-settings';

export function CodexCompactionStatus({ state }: { state: CodexSettingsState }) {
  if (!state.settings.override_model_catalog && state.needs_sync.length) return <p role="alert" className="error-feedback">覆盖偏好已关闭，但部分目标尚未同步。请处理下方待同步目标后再重启 Codex。</p>;
  if (!state.settings.override_model_catalog) return <p role="status">模型能力覆盖已关闭；模型列表照常同步，Anchor 不配置目录内的压缩阈值，保留 Codex 自身设置。</p>;
  if (!state.compaction.length) return <p role="status">尚无已启用的 Codex 托管目标，自动压缩尚未配置。</p>;
  return <div aria-label="模型压缩配置" className="space-y-2">
    <p className="text-sm text-muted-foreground">以下核对的是配置文件；是否已被 Codex 加载、是否实际触发压缩，尚未验证。</p>
    <ul className="max-h-64 space-y-2 overflow-y-auto text-sm">{state.compaction.map(row => {
      const title = `${row.binding_id} / ${row.model_id}${row.is_default ? '（默认模型）' : ''}`;
      if (row.status === 'written') return <li key={`${row.binding_id}/${row.model_id}`}>{title}：窗口 {row.context_window!.toLocaleString('en-US')}，压缩阈值 {row.token_limit!.toLocaleString('en-US')} tokens；已写入，需重启 Codex 加载。</li>;
      if (row.status === 'disabled') return <li key={`${row.binding_id}/${row.model_id}`}>{title}：未启用 Anchor 自动配置；Codex 自身的自动压缩不受此开关控制。</li>;
      const reason = row.status === 'missing_context' ? '未配置压缩阈值：缺少已保存的上下文窗口。先在模型设置中保存上下文，再同步配置并重启 Codex。'
        : row.status === 'needs_sync' ? '配置待同步，尚未确认窗口和阈值。请在 Agent 管理中更新配置。'
          : row.status === 'unmanaged' ? '尚未应用托管配置，压缩阈值未写入。请在 Agent 管理中应用配置。'
            : '当前绑定没有模型目录，Anchor 无法自动配置压缩阈值。';
      return <li key={`${row.binding_id}/${row.model_id}`} role="alert" className="error-feedback">{title}：{reason}</li>;
    })}</ul>
  </div>;
}

export function CodexSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const enabled = !prototype && coreReady;
  const c = useCodexSettings(enabled, onActivity);
  return <section className="detail-card" aria-labelledby="codex-settings-title">
    <header className="card-toolbar"><h2 id="codex-settings-title">Codex</h2></header>
    {!enabled ? <p role="status">{prototype ? '桌面专用' : '未连接'}</p> : <form className="space-y-4" onSubmit={e => { e.preventDefault(); void c.save(); }}>
      <label className="model-setting-field"><span>Codex 重试次数</span><input className="input-field" type="number" min="0" max="10" step="1" value={c.draft} disabled={c.busy || !c.state || c.needsRead} onChange={e => c.setDraft(e.target.value)} /></label>
      <label className="model-setting-field"><span>允许 Anchor 覆盖模型能力与自动压缩</span><Switch aria-describedby="codex-catalog-impact" checked={c.overrideCatalog} disabled={c.busy || !c.state || c.needsRead} onCheckedChange={c.setOverrideCatalog} /></label>
      <p id="codex-catalog-impact" className="text-sm text-muted-foreground">默认关闭。模型列表始终随托管同步，不受此开关影响；有 Codex 原生模板的模型保留原生提示词、personality 和工具能力。开启并保存后，才把模型设置中的上下文、图像输入、并行工具和思考档位写入目录，并按下方设置配置自动压缩。</p>
      <p className="text-sm text-muted-foreground">Codex 要求目录中每个模型都带提示词，没有原生模板的映射模型（例如 Claude）始终使用 Anchor 的通用编程助手提示词。项目 AGENTS.md 和用户自定义指令保持不变。两种操作都需要重启 Codex，已有会话仍可能保留旧配置。</p>
      <label className="model-setting-field"><span>自动配置上下文压缩</span><Switch checked={c.autoCompact} disabled={c.busy || !c.state || c.needsRead || !c.overrideCatalog} onCheckedChange={c.setAutoCompact} /></label>
      <label className="model-setting-field"><span>压缩阈值（上下文的 %）</span><input className="input-field" type="number" min="1" max="99" step="1" value={c.compactPercent} disabled={c.busy || !c.state || c.needsRead || !c.autoCompact || !c.overrideCatalog} onChange={e => c.setCompactPercent(e.target.value)} /></label>
      <p className="text-sm text-muted-foreground">按已保存的模型窗口和设定比例生成压缩阈值。缺少窗口时无法配置，需先保存模型上下文；保存后重启 Codex 加载。</p>
      <p className="text-sm text-muted-foreground">开启后按模型配置，并移除已有全局窗口和压缩阈值覆盖；关闭时恢复原值。</p>
      {c.state && <CodexCompactionStatus state={c.state} />}
      {!c.valid && <p role="alert">参数无效</p>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
      {c.notice && <p role="status">{c.notice}</p>}
      {!!c.state?.needs_sync.length && <p role="status">待同步：{c.state.needs_sync.join('、')}</p>}
      <div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.read()}>读取</Button><Button type="submit" disabled={c.busy || !c.state || !c.valid || c.needsRead}>{c.busy ? '处理中' : '保存'}</Button></div>
    </form>}
  </section>;
}
