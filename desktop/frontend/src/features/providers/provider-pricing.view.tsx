import { useEffect, useState } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import type { Price, Workspace } from './provider-features.model';
import type { ProviderFeatures } from './use-provider-features';
export function ProviderPricingView({ w, f, disabled }: { w: Workspace; f: ProviderFeatures; disabled: boolean }) {
  const [alias, setAlias] = useState('');
  const [catalogProvider, setCatalogProvider] = useState('');
  const [catalogModel, setCatalogModel] = useState('');
  const [input, setInput] = useState(''); const [output, setOutput] = useState('');
  const [read, setRead] = useState(''); const [write, setWrite] = useState(''); const [source, setSource] = useState('custom');
  function populate(p?: Price) { setInput(p ? String(p.input_usd) : ''); setOutput(p ? String(p.output_usd) : ''); setRead(p?.cache_read_usd == null ? '' : String(p.cache_read_usd)); setWrite(p?.cache_write_usd == null ? '' : String(p.cache_write_usd)); setSource(p?.source ?? 'custom'); }
  useEffect(() => { if (w.imported_price) { setAlias(w.imported_price.model); populate(w.imported_price); } }, [w.imported_price]);
  const valid = alias && input !== '' && output !== '' && [input, output, read, write].every(v => v === '' || (Number.isFinite(Number(v)) && Number(v) >= 0 && Number(v) <= 1e6));
  return <div className="feature-form"><h3>模型单价（USD / 百万 token）</h3>
    <label htmlFor="price-alias">公开模型</label><select id="price-alias" value={alias} disabled={disabled} onChange={e => { setAlias(e.target.value); setCatalogModel(w.models.find(m => m.id === e.target.value)?.upstream ?? ''); populate(w.prices.find(p => p.model === e.target.value)); }}><option value="">选择模型映射</option>{[...new Set(w.models.map(m => m.id))].map(id => <option key={id}>{id}</option>)}</select>
    <label htmlFor="price-provider">models.dev 供应商 ID</label><Input id="price-provider" placeholder="例如 openai、anthropic" value={catalogProvider} disabled={disabled} onChange={e => setCatalogProvider(e.target.value)} />
    <label htmlFor="price-catalog-model">models.dev 模型 ID</label><Input id="price-catalog-model" value={catalogModel} disabled={disabled} onChange={e => setCatalogModel(e.target.value)} />
    <Button variant="outline" size="sm" disabled={disabled || !alias || !catalogProvider || !catalogModel} onClick={() => void f.run('price-import', { target: alias, model: catalogModel, catalog_provider: catalogProvider })}>读取参考价格</Button>
    <form className="feature-form" onSubmit={e => { e.preventDefault(); if (valid) void f.run('price-put', { target: alias, price: { model: alias, input_usd: Number(input), output_usd: Number(output), cache_read_usd: read === '' ? null : Number(read), cache_write_usd: write === '' ? null : Number(write), source, updated_at: 0 } }); }}>
      {([{ id: 'in', name: '普通输入', value: input, set: setInput }, { id: 'out', name: '输出', value: output, set: setOutput }, { id: 'read', name: '缓存读取（留空为未知）', value: read, set: setRead }, { id: 'write', name: '缓存写入（留空为未知）', value: write, set: setWrite }]).map(field => <label key={field.id} htmlFor={`price-${field.id}`}>{field.name}<Input id={`price-${field.id}`} type="number" min="0" max="1000000" step="any" value={field.value} disabled={disabled || !alias} onChange={e => { field.set(e.target.value); setSource('custom'); }} /></label>)}
      <p className="detail-footnote break-all">来源：{source}</p><Button size="sm" disabled={disabled || !valid}>确认保存单价</Button>
    </form>
  </div>;
}
