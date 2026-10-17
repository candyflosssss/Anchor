import { useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import { Button } from '../../components/ui/button';
import type { ProviderTransport } from '../providers/provider.model';
import type { ModelGroup } from '../providers/provider-summary.model';
import { useModelSpeedSettings } from './use-model-speed-settings';

export function ModelSpeedSettingsDialog({ group, active, transport, close, saved, capabilities }: {
  group: ModelGroup; active: boolean; transport: ProviderTransport; close: () => void; saved: () => Promise<void>; capabilities: () => void;
}) {
  const sources = [...new Set(group.candidates.map(c => c.provider_id))];
  const [provider, setProvider] = useState(sources.length === 1 ? sources[0] : '');
  const c = useModelSpeedSettings(group.alias, provider, active, transport, saved);
  const locked = !active || c.busy || c.needsRead || !c.workspace || !group.complete;
  const canCreate = !c.tier && !/\((fast|ultrafast)\)$/.test(group.alias);
  return <Dialog open={active} onOpenChange={open => { if (!open && !c.busy) close(); }}><DialogContent className="model-settings-dialog" onRequestClose={() => { if (!c.busy) close(); }}>
    <header><DialogTitle>模型设置 · {group.alias}</DialogTitle><DialogDescription>直接配置 Fast / Ultrafast。速度设置与重试次数、上下文、能力和思考参数分开保存，切换页面不保留未保存草稿。</DialogDescription></header>
    <nav className="settings-actions" aria-label="模型设置分类"><Button variant="secondary" disabled>速度与分身</Button><Button variant="outline" disabled={c.busy} onClick={capabilities}>重试、上下文与能力</Button></nav>
    {c.error && <p className="feedback error-feedback" role="alert">{c.error}</p>}{c.notice && <p className="feedback" role="status">{c.notice}</p>}
    <div className="model-setting-field"><label htmlFor="model-speed-source">分身来源渠道</label><select id="model-speed-source" className="select-field" value={provider} disabled={c.busy} onChange={e => setProvider(e.target.value)}><option value="">请选择供应商渠道</option>{sources.map(id => <option key={id} value={id}>{id} · {group.candidates.find(c => c.provider_id === id)?.upstream}</option>)}</select></div>
    <p className="card-hint">固定档位对当前公开模型的所有渠道生效；新分身只复制所选供应商渠道，不复制其他渠道。</p>
    {provider && <Button variant="outline" disabled={c.busy} onClick={() => void c.read()}>重新读取速度设置</Button>}
    {!provider ? <p className="card-hint">此模型有多个供应商渠道，请先选择来源，再核对当前配置。</p> : !c.workspace ? <p className="card-hint">{c.busy ? '正在读取速度设置…' : '未取得速度设置，请重新读取。'}</p> : <form onSubmit={e => { e.preventDefault(); if (!locked) void c.save(); }}>
      <fieldset disabled={locked} className="model-settings-fields">
        <section className="settings-section"><h3>当前映射的固定档位</h3><div className="model-setting-field"><label htmlFor="model-speed-tier">Anchor 固定速度档位</label><select id="model-speed-tier" className="select-field" value={c.tier} onChange={e => c.changeTier(e.target.value)}><option value="">不固定（沿用请求参数）</option><option value="priority">Fast（priority）</option><option value="ultrafast">Ultrafast</option></select></div>
          <p className="card-hint">修改这里会改变 {group.alias} 本身，不会额外创建模型。固定后由 Anchor 覆盖请求的 service_tier，上游模型 ID 保持不变。</p></section>
        <section className="settings-section"><h3>额外创建速度分身（可选）</h3>
          {canCreate ? <>{['fast', 'ultrafast'].map(speed => <label className="checkbox-row" key={speed}><input type="checkbox" checked={c.variants.includes(speed)} onChange={e => c.toggleVariant(speed, e.target.checked)} />{speed === 'fast' ? 'Fast' : 'Ultrafast'} · {group.alias}({speed})</label>)}<p className="card-hint">保留普通映射；勾选项与当前档位一起保存，名称冲突时整体不保存。分身是独立快照，后续编辑不会自动同步，不继承普通档位参考价格。</p></> : <p className="card-hint">仅不固定档位、无速度后缀的普通映射可以创建分身。若要保留普通模型，请保持上面的档位为「不固定」。</p>}
        </section>
        <p className="card-hint">需要上游支持，可能增加费用；保存成功不代表实际加速。不会解锁 Codex 原生速度菜单，也不会发送付费推理探测。</p>
        {c.validation && <p className="feedback error-feedback" role="alert">{c.validation}</p>}
      </fieldset>
      <footer className="settings-actions"><Button type="button" variant="outline" disabled={c.busy} onClick={close}>关闭</Button><Button disabled={locked || !!c.validation}>保存速度设置</Button></footer>
    </form>}
  </DialogContent></Dialog>;
}
