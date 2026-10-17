import { useState } from 'react';
import { Dialog, DialogContent } from '../../components/ui/dialog';
import { Button } from '../../components/ui/button';
import type { ProviderTransport } from '../providers/provider.model';
import type { ModelGroup } from '../providers/provider-summary.model';
import { useModelSpeedSettings } from './use-model-speed-settings';
import { ModelSettingsHeader } from './model-settings-header.view';

export function ModelSpeedSettingsDialog({ group, active, transport, close, saved, capabilities }: {
  group: ModelGroup; active: boolean; transport: ProviderTransport; close: () => void; saved: () => Promise<void>; capabilities: () => void;
}) {
  const sources = [...new Set(group.candidates.map(c => c.provider_id))];
  const [provider, setProvider] = useState(sources.length === 1 ? sources[0] : '');
  const c = useModelSpeedSettings(group.alias, provider, active, transport, saved);
  const locked = !active || c.busy || c.needsRead || !c.workspace || !group.complete;
  const canCreate = !c.tier && !/\((fast|ultrafast)\)$/.test(group.alias);
  return <Dialog open={active} onOpenChange={open => { if (!open && !c.busy) close(); }}><DialogContent className="model-settings-dialog" onRequestClose={() => { if (!c.busy) close(); }}>
    <ModelSettingsHeader alias={group.alias} page="speed" busy={c.busy} switchPage={capabilities} />
    <div className="model-speed-body">
    {c.error && <p className="feedback error-feedback" role="alert">{c.error}</p>}{c.notice && <p className="feedback" role="status">{c.notice}</p>}
    <div className="model-setting-field"><label htmlFor="model-speed-source">分身来源渠道</label><select id="model-speed-source" className="select-field" value={provider} disabled={c.busy} onChange={e => setProvider(e.target.value)}><option value="">请选择供应商渠道</option>{sources.map(id => <option key={id} value={id}>{id} · {group.candidates.find(c => c.provider_id === id)?.upstream}</option>)}</select></div>

    {provider && <Button variant="outline" disabled={c.busy} onClick={() => void c.read()}>重新读取速度设置</Button>}
    {!provider ? <p className="card-hint">请选择来源渠道。</p> : !c.workspace ? <p className="card-hint">{c.busy ? '正在读取速度设置…' : '未取得速度设置，请重新读取。'}</p> : <form onSubmit={e => { e.preventDefault(); if (!locked) void c.save(); }}>
      <fieldset disabled={locked} className="model-settings-fields">
        <section className="settings-section"><h3>固定速度档位</h3><div className="model-setting-field"><label htmlFor="model-speed-tier">Anchor 固定速度档位</label><select id="model-speed-tier" className="select-field" value={c.tier} onChange={e => c.changeTier(e.target.value)}><option value="">不固定（沿用请求参数）</option><option value="priority">Fast（priority）</option><option value="ultrafast">Ultrafast</option></select></div>
          <p className="card-hint">应用于当前模型的所有渠道，不创建新模型。</p></section>
        <section className="settings-section"><h3>创建分身</h3>
          {canCreate ? <>{['fast', 'ultrafast'].map(speed => <label className="checkbox-row" key={speed}><input type="checkbox" checked={c.variants.includes(speed)} onChange={e => c.toggleVariant(speed, e.target.checked)} />{speed === 'fast' ? 'Fast' : 'Ultrafast'} · {group.alias}({speed})</label>)}<p className="card-hint">仅复制所选渠道，保留原模型。</p></> : <p className="card-hint">仅普通模型在「不固定」档位下可创建分身。</p>}
        </section>
        <p className="card-hint">需要上游支持，可能增加费用；保存不代表已加速。</p>
        <details className="model-settings-help"><summary>使用说明</summary><p>固定档位覆盖请求的 service_tier，上游模型 ID 不变。分身为独立快照，后续修改不自动同步，也不继承普通档位价格；名称冲突时整体不保存。</p><p>不会解锁 Codex 原生速度菜单或发送付费推理探测。切换分类前请保存，未保存的修改会丢弃。</p></details>
        {c.validation && <p className="feedback error-feedback" role="alert">{c.validation}</p>}
      </fieldset>
      <footer className="model-settings-footer"><div className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={close}>关闭</Button><Button disabled={locked || !!c.validation}>保存速度设置</Button></div></footer>
    </form>}
    </div>
  </DialogContent></Dialog>;
}
