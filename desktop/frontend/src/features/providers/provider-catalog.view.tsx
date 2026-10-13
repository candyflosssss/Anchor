import { Download, LoaderCircle, Plus } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { catalogFailureText, catalogProblemText, catalogRateFailureText, protocolName } from './provider.model';
import type { Workspace } from './provider-features.model';
import type { ProviderController } from './use-provider-controller';

export function ProviderCatalogView({ c, w, disabled }: { c: ProviderController; w: Workspace; disabled: boolean }) {
  const models = c.models;
  const ids = [...new Set([...(models?.models ?? []), ...w.keys.flatMap(k => k.model_protocols.map(m => m.model))])];
  const mapped = new Map(w.models.map(m => [m.upstream, m.id]));
  const rateLabel = (id: string) => {
    const k = models?.keys.find(k => k.id === id);
    if (!k) return '分组倍率尚未查询';
    const group = k.group_name ? `${k.group_name}（${k.group}）` : k.group;
    return `${group ? `分组 ${group} · ` : ''}${k.multiplier !== null ? `${k.multiplier}×${k.rate_basis === 'personal_override' ? '（个人倍率）' : ''}` : `倍率未知：${catalogRateFailureText[k.rate_failure] || '上游未提供'}`}`;
  };
  return <section className="detail-card model-directory" aria-label="上游模型目录" aria-busy={c.modelsBusy}>
    <div className="card-toolbar"><h2>Key 的模型与协议</h2><Button variant="outline" disabled={disabled} onClick={() => void c.discoverModels()}>{c.modelsBusy ? <LoaderCircle className="loading-icon" /> : <Download />}{c.modelsBusy ? '正在获取目录…' : '获取模型目录'}</Button></div>
    <p className="card-hint">自动查询全部 {w.keys.length} 个 Key（含已停用 Key），相同模型合并显示。无 Key 时使用匿名请求。</p>
    {c.modelsProblem && <p className="catalog-feedback" role="alert">{catalogProblemText(c.modelsProblem)}</p>}
    <p className="catalog-feedback" aria-live="polite">{models ? `${models.failure ? '目录获取失败' : `已获取 ${models.models.length} 个模型${models.partial ? '（部分 Key 查询失败）' : ''}`} · ${models.elapsed_ms} ms · revision ${models.revision}` : ''}</p>
    {models?.failure && <p className="catalog-feedback" role="alert">{catalogFailureText[models.failure]}</p>}
    {models && !models.failure && !models.models.length && <p className="catalog-feedback">上游返回空目录。</p>}
    {models && <ul className="catalog-key-summary" aria-label="Key 查询结果">{models.keys.map(k => <li key={k.id}>
      <strong>Key {k.id}{!k.enabled && '（已停用）'}</strong><span>{rateLabel(k.id)}</span>
      <span className="cell-hint">{k.failure ? catalogFailureText[k.failure] : `${models.entries.filter(e => e.key_ids.includes(k.id)).length} 个模型`} · HTTP {k.http_status || '—'}</span>
    </li>)}</ul>}
    {ids.length > 0 && <ul className="catalog-list catalog-merged" tabIndex={0} aria-label="上游模型 ID">{ids.map(id => {
      const listed = models?.entries.find(e => e.id === id)?.key_ids ?? [];
      const keyIDs = [...new Set([...listed, ...w.keys.filter(k => k.model_protocols.some(m => m.model === id)).map(k => k.id)])];
      return <li key={id}><div className="catalog-model-heading"><strong className="mono break-all">{id}</strong>
        {mapped.has(id) ? <span className="cell-hint">已映射为 {mapped.get(id)}</span> : <Button size="sm" variant="ghost" disabled={disabled} aria-label={`导入 ${id}`} onClick={() => c.openDialog({ kind: 'mapping', upstream: id })}><Plus />导入</Button>}
      </div><div className="catalog-model-keys">{keyIDs.map(keyID => {
        const key = w.keys.find(k => k.id === keyID);
        return <div key={keyID} className="catalog-model-key"><div><strong>Key {keyID}{key && !key.enabled && '（已停用）'}</strong><span className="cell-hint">{listed.includes(keyID) ? '目录列出' : '仅已存协议'} · {rateLabel(keyID)}</span></div>
          {key && <select className="select-field" aria-label={`${id} · Key ${keyID} 的协议`} value={key.model_protocols.find(m => m.model === id)?.protocol ?? 'inherit'} disabled={disabled || c.features.needsRead} onChange={e => void c.features.run('key-model-protocol', { target: key.id, model: id, protocol: e.target.value })}>
            <option value="inherit">继承 Key（{protocolName[key.protocol as keyof typeof protocolName]}）</option>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>}
        </div>;
      })}</div></li>;
    })}</ul>}
  </section>;
}
