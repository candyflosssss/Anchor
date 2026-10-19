import { Button } from '../../components/ui/button';
import { prototype } from '../../desktop/environment';
import { useCodexSettings } from './use-codex-settings';

export function CodexSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const enabled = !prototype && coreReady;
  const c = useCodexSettings(enabled, onActivity);
  return <section className="detail-card" aria-labelledby="codex-settings-title">
    <header className="card-toolbar"><h2 id="codex-settings-title">Codex</h2></header>
    <p className="card-hint">统一控制 Codex 的请求重试和流中断重试，适用于当前核心托管的所有 Codex 模型。默认 5 次，不含首次请求；0 表示关闭重试。修改后需重启 Codex。</p>
    {!enabled ? <p role="status">{prototype ? 'Codex 重试设置仅在桌面模式提供。' : '请先连接核心以读取和修改 Codex 设置。'}</p> : <form className="space-y-4" onSubmit={e => { e.preventDefault(); void c.save(); }}>
      <label className="model-setting-field"><span>Codex 重试次数</span><input className="input-field" type="number" min="0" max="10" step="1" value={c.draft} disabled={c.busy || !c.state || c.needsRead} onChange={e => c.setDraft(e.target.value)} /></label>
      {!c.valid && <p role="alert">重试次数须为 0–10 的整数。</p>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
      {c.notice && <p role="status">{c.notice}</p>}
      {!!c.state?.needs_sync.length && <p role="status">需同步的 Codex：{c.state.needs_sync.join('、')}。请前往「Agents」检查配置并同步，然后重启 Codex。</p>}
      <div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.read()}>重新读取 Codex 设置</Button><Button type="submit" disabled={c.busy || !c.state || !c.valid || c.needsRead}>{c.busy ? '处理中…' : '保存 Codex 设置'}</Button></div>
    </form>}
  </section>;
}
