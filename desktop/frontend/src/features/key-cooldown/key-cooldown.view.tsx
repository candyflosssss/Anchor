import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { prototype } from '../../desktop/environment';
import { useKeyCooldown } from './use-key-cooldown';

export function KeyCooldownSettings({ coreReady, onActivity }: { coreReady: boolean; onActivity: (busy: boolean) => void }) {
  const enabled = !prototype && coreReady;
  const c = useKeyCooldown(enabled, onActivity);
  const disabled = c.busy || !c.state || c.needsRead;
  return <section className="detail-card" aria-labelledby="key-cooldown-title">
    <header className="card-toolbar"><h2 id="key-cooldown-title">Key 冷却</h2></header>
    {!enabled ? <p role="status">{prototype ? '桌面专用' : '未连接'}</p> : <form className="space-y-4" onSubmit={e => { e.preventDefault(); void c.save(); }}>
      <label className="model-setting-field"><span>自动冷却</span><Switch aria-label="自动冷却" checked={c.automatic} disabled={disabled} onCheckedChange={c.setAutomatic} /></label>
      <label className="model-setting-field"><span>错误有效期（秒）</span><input className="input-field" type="number" min="1" max="3600" step="1" value={c.windowSeconds} disabled={disabled || !c.automatic} onChange={e => c.setWindowSeconds(e.target.value)} /></label>
      <label className="model-setting-field"><span>错误分数阈值</span><input className="input-field" type="number" min="1" max="100" step="1" value={c.threshold} disabled={disabled || !c.automatic} onChange={e => c.setThreshold(e.target.value)} /></label>
      <label className="model-setting-field"><span>冷却时长（秒）</span><input className="input-field" type="number" min="1" max="3600" step="1" value={c.seconds} disabled={disabled || !c.automatic} onChange={e => c.setSeconds(e.target.value)} /></label>
      <label className="model-setting-field"><span>429 冷却时长（秒）</span><input className="input-field" type="number" min="1" max="3600" step="1" value={c.rateSeconds} disabled={disabled || !c.automatic} onChange={e => c.setRateSeconds(e.target.value)} /></label>
      {!c.valid && <p role="alert">参数无效</p>}
      {c.error && <p role="alert" className="error-feedback">{c.error}</p>}
      {c.notice && <p role="status">{c.notice}</p>}
      <div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.read()}>读取</Button><Button type="submit" disabled={disabled || !c.valid}>{c.busy ? '处理中' : '保存'}</Button></div>
    </form>}
  </section>;
}
