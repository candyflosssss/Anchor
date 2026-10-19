import { useState } from 'react';
import { Check, ChevronDown, ChevronLeft, ChevronRight, Download, Layers, LoaderCircle, Plus, Search } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { catalogFailureText, catalogProblemText, catalogRateFailureText, protocolName } from './provider.model';
import { catalogImportPlan, type Workspace } from './provider-features.model';
import type { ProviderController } from './use-provider-controller';
import './provider-catalog.css';

const pageSize = 8;

export function ProviderCatalogView({ c, w, disabled, setup = false }: { c: ProviderController; w: Workspace; disabled: boolean; setup?: boolean }) {
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState('all');
  const [page, setPage] = useState(0);
  const models = c.models;
  const ids = [...new Set([...(models?.models ?? []), ...w.keys.flatMap(k => k.model_protocols.map(m => m.model))])];
  const mapped = new Map(w.models.map(m => [m.upstream, m.id]));
  const importPlan = catalogImportPlan(ids, w);
  const filtered = ids.filter(id => id.toLowerCase().includes(query.trim().toLowerCase()) && (filter === 'all' || mapped.has(id) === (filter === 'mapped')));
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pages - 1);
  const visible = filtered.slice(currentPage * pageSize, (currentPage + 1) * pageSize);
  const enabledKeys = w.keys.filter(k => k.enabled).length;
  const rateLabel = (id: string) => {
    const k = models?.keys.find(k => k.id === id);
    const cached = w.keys.find(k => k.id === id);
    if (cached?.rate?.multiplier != null) {
      const rate = cached.rate;
      return `${rate.group_name || rate.group ? `分组 ${rate.group_name || rate.group} · ` : ''}分组倍率 ${rate.multiplier}× · 相对倍率 ${cached.effective_multiplier ?? '未知'}× · 缓存${rate.failure ? '（刷新失败，沿用上次结果）' : ''}`;
    }
    if (!k) return '分组倍率未知（未缓存）';
    const group = k.group_name ? `${k.group_name}（${k.group}）` : k.group;
    return `${group ? `分组 ${group} · ` : ''}${k.multiplier !== null ? `${k.multiplier}×${k.rate_basis === 'personal_override' ? '（个人倍率）' : k.rate_basis === 'key_billing' ? '（当前计费）' : ''}` : `倍率未知：${catalogRateFailureText[k.rate_failure] || '上游未提供'}`}`;
  };
  return <section className="detail-card model-directory" aria-label="上游模型目录" aria-busy={c.modelsBusy}>
    <div className="card-toolbar catalog-toolbar">
      <div className="catalog-title"><h2>模型目录</h2>{models && !models.failure && <span className="catalog-count">{models.models.length}</span>}</div>
      <div className="catalog-toolbar-actions">{ids.length > 0 && <Button disabled={disabled || c.features.needsRead || !importPlan.ready.length} onClick={() => void c.features.importModels(ids)}><Plus />全部导入（{importPlan.ready.length}）</Button>}<Button variant="outline" disabled={disabled} onClick={() => void c.discoverModels()}>{c.modelsBusy ? <LoaderCircle className="loading-icon" /> : <Download />}{c.modelsBusy ? '正在获取目录…' : '获取模型目录'}</Button></div>
    </div>
    {c.features.importStatus && <p className="catalog-import-status" role="status">{c.features.importStatus}</p>}
    <div className="catalog-status">
      <span>{w.keys.length ? `${enabledKeys} 个启用 Key` : '未保存 Key · 使用匿名请求'}</span>
      <span aria-live="polite">{models ? `${models.failure ? '目录获取失败' : `已获取 ${models.models.length} 个模型${models.partial ? '（部分 Key 查询失败）' : ''}`} · ${models.elapsed_ms} ms` : '尚未获取目录'}</span>
    </div>
    {c.modelsProblem && <p className="catalog-feedback" role="alert">{catalogProblemText(c.modelsProblem)}</p>}
    {models?.failure && <p className="catalog-feedback" role="alert">{catalogFailureText[models.failure]}</p>}
    {models && <details className="catalog-diagnostics">
      <summary><ChevronDown aria-hidden="true" />Key 查询详情<span>{models.keys.length} 个 Key{models.partial ? ' · 部分失败' : ''}</span></summary>
      <p className="cell-hint">revision {models.revision}</p>
      <ul className="catalog-key-summary" aria-label="Key 查询结果">{models.keys.map(k => <li key={k.id}>
        <strong>Key {k.id}{!k.enabled && '（已停用）'}</strong><span>{rateLabel(k.id)}</span>
        <span className="cell-hint">{k.failure ? catalogFailureText[k.failure] : `${models.entries.filter(e => e.key_ids.includes(k.id)).length} 个模型`} · HTTP {k.http_status || '—'}</span>
      </li>)}</ul>
    </details>}
    {ids.length > 0 ? <>
      <div className="catalog-filters">
        <div className="catalog-search"><Search aria-hidden="true" /><Input type="search" aria-label="搜索模型目录" placeholder="搜索模型名称…" value={query} onChange={e => { setQuery(e.target.value); setPage(0); }} /></div>
        <select className="select-field" aria-label="模型目录映射状态" value={filter} onChange={e => { setFilter(e.target.value); setPage(0); }}>
          <option value="all">全部映射状态</option><option value="unmapped">未映射</option><option value="mapped">已映射</option>
        </select>
      </div>
      {filtered.length ? <div className="catalog-table">
        <div className="catalog-columns" aria-hidden="true"><span>上游模型</span><div><span>可用 Key / 分组倍率 / 相对倍率</span><span>请求协议</span></div><span>映射</span></div>
        <ul className="catalog-list" aria-label="上游模型 ID">{visible.map(id => {
          const listed = (models?.entries.find(e => e.id === id)?.key_ids ?? []).filter(keyID => keyID === 'anonymous' || w.keys.some(k => k.id === keyID && k.enabled));
          const keyIDs = [...new Set([...listed, ...w.keys.filter(k => k.model_protocols.some(m => m.model === id)).map(k => k.id)])];
          return <li key={id} className="catalog-model-row">
            <div className="catalog-model-heading"><strong className="mono break-all">{id}</strong>
              <span className="cell-hint">{listed.length ? `${listed.length} 个可用 Key` : '目录尚未确认可选 Key'}</span>
              {mapped.has(id) && <span className="cell-hint break-all">映射为 {mapped.get(id)}</span>}
            </div>
            <div className="catalog-model-keys" aria-label={`${id} 的可选 Key`}>{keyIDs.length ? keyIDs.map(keyID => {
              const key = w.keys.find(k => k.id === keyID);
              return <div key={keyID} className="catalog-model-key"><div className="catalog-key-info"><strong>{keyID === 'anonymous' ? '匿名请求' : `Key ${keyID}`}{key && !key.enabled && <span className="catalog-key-note">已停用</span>}</strong>
                <span className="cell-hint">{!listed.includes(keyID) && '仅已存协议 · '}{rateLabel(keyID)}</span></div>
                {key ? <select className="select-field" aria-label={`${id} · Key ${keyID} 的协议`} value={key.model_protocols.find(m => m.model === id)?.protocol ?? 'inherit'} disabled={disabled || c.features.needsRead} onChange={e => void c.features.run('key-model-protocol', { target: key.id, model: id, protocol: e.target.value })}>
                  <option value="inherit">继承 Key（{protocolName[key.protocol as keyof typeof protocolName]}）</option>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select> : <span className="cell-hint">无需 Key 协议配置</span>}
              </div>;
            }) : <span className="cell-hint">暂无 Key 信息</span>}</div>
            <div className="catalog-model-action">{mapped.has(id) ? <span className="catalog-mapped"><Check aria-hidden="true" />已映射</span> : <><Button size="sm" variant="outline" disabled={disabled || c.features.needsRead || !importPlan.ready.includes(id)} aria-label={`快速导入 ${id}`} onClick={() => void c.features.importModels([id])}><Plus />快速导入</Button>{!setup && <Button size="sm" variant="ghost" disabled={disabled || c.features.needsRead} aria-label={`导入 ${id}`} onClick={() => c.openDialog({ kind: 'mapping', upstream: id })}>自定义</Button>}{importPlan.manual.includes(id) && <span className="cell-hint">需自定义别名</span>}</>}</div>
          </li>;
        })}</ul>
      </div> : <div className="catalog-empty"><Search aria-hidden="true" /><strong>没有匹配的模型</strong><p>试试其他名称，或清除筛选条件。</p><Button variant="outline" size="sm" onClick={() => { setQuery(''); setFilter('all'); setPage(0); }}>清除目录筛选</Button></div>}
      <div className="catalog-pagination"><span aria-live="polite">{filtered.length ? `${currentPage * pageSize + 1}–${Math.min((currentPage + 1) * pageSize, filtered.length)} / ${filtered.length} 个模型` : '0 个模型'}<span className="catalog-total"> · 目录共 {ids.length} 个</span></span>
        {pages > 1 && <div><Button variant="ghost" size="icon" aria-label="模型目录上一页" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}><ChevronLeft /></Button><span>{currentPage + 1} / {pages}</span><Button variant="ghost" size="icon" aria-label="模型目录下一页" disabled={currentPage === pages - 1} onClick={() => setPage(currentPage + 1)}><ChevronRight /></Button></div>}
      </div>
    </> : <div className="catalog-empty"><Layers aria-hidden="true" /><strong>{c.modelsBusy ? '正在读取上游模型' : models ? models.failure ? '暂时无法获取模型目录' : '上游返回空目录。' : '获取可用模型，开始配置'}</strong><p>{models?.failure ? '请根据上方提示检查 Key 或连接后重试。' : '获取目录后，可查看各 Key 的模型、分组倍率与请求协议。'}</p></div>}
  </section>;
}
