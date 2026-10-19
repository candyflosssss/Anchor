import { Button } from '../../components/ui/button';
import { prototype } from '../../desktop/environment';
import { useCodexSettings } from './use-codex-settings';

export function CodexSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const enabled = !prototype && coreReady;
  const c = useCodexSettings(enabled, onActivity);
  return <section className="detail-card" aria-labelledby="codex-settings-title">
    <header className="card-toolbar"><h2 id="codex-settings-title">Codex</h2></header>
    {!enabled ? <p role="status">{prototype ? '桌面专用' : '未连接'}</p> : <form className="space-y-4" onSubmit={e => { e.preventDefault(); void c.save(); }}>
      <label className="model-setting-field"><span>Codex 重试次数</span><input className="input-field" type="number" min="0" max="10" step="1" value={c.draft} disabled={c.busy || !c.state || c.needsRead} onChange={e => c.setDraft(e.target.value)} /></label>
      {!c.valid && <p role="alert">参数无效</p>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
      {c.notice && <p role="status">{c.notice}</p>}
      {!!c.state?.needs_sync.length && <p role="status">待同步：{c.state.needs_sync.join('、')}</p>}
      <div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.read()}>读取</Button><Button type="submit" disabled={c.busy || !c.state || !c.valid || c.needsRead}>{c.busy ? '处理中' : '保存'}</Button></div>
    </form>}
  </section>;
}
