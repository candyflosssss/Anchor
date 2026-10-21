import { ProviderProxyPanel } from '../proxies/proxies.view';
import { useMappingSelection } from './use-provider-mapping-batch';
import { KeyMultiplierView } from './key-multiplier.view';
import { KeyGroupName } from './provider-rate.view';
import { probeFailureText } from './provider-test.model';
import { ProviderCatalogView } from './provider-catalog.view';
import { ProviderGroupRates } from './provider-group-rates.view';
import { ProviderAccountView } from './provider-account.view';
import { useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { ArrowLeft, CircleAlert, CircleCheck, CircleDashed, Copy, KeyRound, List, LoaderCircle, MoreHorizontal, Pencil, Play, Plus, RotateCcw, Trash2, TextCursorInput } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { StatusBadge } from '../../components/shared/status-badge';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { MenuItem, Popover } from '../../components/shared/popover-menu';
import { ProviderPricingView } from './provider-pricing.view';
import { healthText, type Action, type Probe, type Workspace } from './provider-features.model';
import { protocolName, type Provider } from './provider.model';
import { formatAgo, type DetailTab, type ProviderSummary, type Tone } from './provider-summary.model';
import type { ProviderController } from './use-provider-controller';

const tabs: [DetailTab, string][] = [['overview', '概况'], ['keys', 'Key'], ['models', '模型'], ['usage', '用量'], ['account', '账户与余额']];
type Ask = { action: string; title: string; body: string; fields: Partial<Action> };

export function HealthPill({ tone, label }: { tone: Tone; label: string }) {
  return <span className={`health-pill tone-${tone}`}><span className="mini-dot" aria-hidden="true" />{label}</span>;
}

export function ProbeCard({ probe }: { probe: Probe }) {
  return <div className={`probe-result ${probe.failure ? 'failed' : 'passed'}`}>
    <strong>{probe.failure ? '推理测试失败' : '推理测试成功'}</strong>
    <span> · HTTP {probe.http_status || '—'} · {probe.elapsed_ms} ms · Key {probe.key_id || '匿名'}</span>
    <div className="mono break-all">{probe.model} · revision {probe.revision} · {formatAgo(probe.at)}</div>
    {probe.failure && <div>失败原因：{probeFailureText(probe.failure)}{probe.failure === 'unexpected_html' && `（${probe.failure}）`}</div>}
  </div>;
}

export function ProviderDetail({ c, summary, onEdit }: { c: ProviderController; summary: ProviderSummary | undefined; onEdit: () => void }) {
  const p = c.selected as Provider;
  const f = c.features;
  const w = f.data;
  const blocked = c.blocked || !!c.dialog;
  const [ask, setAsk] = useState<Ask | null>(null);
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  function tabKeys(e: KeyboardEvent) {
    const at = tabs.findIndex(([id]) => id === c.detailTab);
    const next = e.key === 'ArrowRight' ? (at + 1) % tabs.length : e.key === 'ArrowLeft' ? (at - 1 + tabs.length) % tabs.length : e.key === 'Home' ? 0 : e.key === 'End' ? tabs.length - 1 : -1;
    if (next < 0) return;
    e.preventDefault(); c.setDetailTab(tabs[next][0]); tabRefs.current[tabs[next][0]]?.focus();
  }
  return <section className="provider-page" aria-label={`${p.id} 的详情`} aria-busy={c.busy}>
    <Button variant="ghost" size="sm" className="back-link" disabled={c.blocked} onClick={() => c.select(null)}><ArrowLeft />返回供应商列表</Button>
    <header className="detail-hero">
      <span className="provider-avatar large" aria-hidden="true">{Array.from(p.id)[0]}</span>
      <div className="detail-title">
        <h1>{p.id}</h1>
        <div className="detail-meta"><StatusBadge enabled={p.enabled} />{summary && <HealthPill tone={summary.tone} label={summary.label} />}</div>
        <span className="provider-url mono" title={p.base_urls.join('\n')}>{p.base_urls[0]}{p.base_urls.length > 1 ? ` +${p.base_urls.length - 1}` : ''}</span>
      </div>
      <div className="detail-actions">
        <Button disabled={blocked || !w} onClick={() => c.openDialog({ kind: 'test' })}><Play />测试连通</Button>
        <Button variant="outline" disabled={blocked} onClick={onEdit}><Pencil />编辑配置</Button>
        <label className="key-toggle" data-state={p.enabled ? 'checked' : 'unchecked'}><Switch className="key-toggle-switch" checked={p.enabled} disabled={blocked || !!c.editor} aria-label={`供应商 ${p.id} 启用状态`} onCheckedChange={() => c.toggleProvider(p)} /><span>{p.enabled ? '已启用' : '已停用'}</span></label>
        <Popover menu label={`${p.id} 的更多操作`} disabled={blocked || !w} trigger={props => <Button variant="ghost" size="icon" aria-label="更多操作" {...props}><MoreHorizontal /></Button>}>
          <MenuItem onSelect={() => c.openDialog({ kind: 'copy' })}><Copy size={14} />复制为新供应商</MenuItem>
          <MenuItem onSelect={() => c.openDialog({ kind: 'rename' })}><TextCursorInput size={14} />改名</MenuItem>
          <MenuItem destructive onSelect={() => c.openDialog({ kind: 'delete' })}><Trash2 size={14} />删除供应商</MenuItem>
        </Popover>
      </div>
    </header>
    {c.detailTab !== 'account' && !c.editor && <ProviderGroupRates key={`${p.id}:${p.revision}`} c={c} />}
    <div className="detail-tabs" role="tablist" aria-label="供应商详情分区" onKeyDown={tabKeys}>
      {tabs.map(([id, title]) => <button key={id} ref={node => { tabRefs.current[id] = node; }} role="tab" id={`tab-${id}`} aria-controls={`panel-${id}`} aria-selected={c.detailTab === id} tabIndex={c.detailTab === id ? 0 : -1} onClick={() => c.setDetailTab(id)}>
        {title}{id === 'keys' && w ? <span className="tab-count">{w.keys.length}</span> : id === 'models' && w ? <span className="tab-count">{w.models.length}</span> : null}
      </button>)}
    </div>
    <div role="tabpanel" id={`panel-${c.detailTab}`} aria-labelledby={`tab-${c.detailTab}`} className="detail-panel" tabIndex={0}>
      {f.error && !c.dialog && <div className="feedback error-feedback" role="alert"><CircleAlert size={16} /><span>{f.error}</span><Button variant="outline" size="sm" disabled={f.busy} onClick={() => void (f.needsRead ? c.load() : f.refresh())}>重新读取</Button></div>}
      {c.detailTab === 'account' ? <ProviderAccountView c={c} /> : !w ? (!f.error && <div className="empty-state compact" role="status"><LoaderCircle className="loading-icon" size={24} /><p>正在读取 Key、模型与用量…</p></div>)
        : c.detailTab === 'overview' ? <><OverviewTab c={c} p={p} w={w} /><ProviderProxyPanel key={p.id} providerId={p.id} disabled={blocked} /></>
        : c.detailTab === 'keys' ? <><KeysTab c={c} w={w} disabled={blocked} ask={setAsk} /><ProviderProxyPanel key={p.id} providerId={p.id} disabled={blocked} /></>
        : c.detailTab === 'models' ? <ModelsTab key={w.provider_id} c={c} w={w} disabled={blocked} ask={setAsk} />
        : <UsageTab c={c} w={w} disabled={blocked} />}
    </div>
    <ConfirmAction open={!!ask} title={ask?.title ?? ''} busy={f.busy} onCancel={() => setAsk(null)} onConfirm={() => { if (ask) void f.run(ask.action, { ...ask.fields, approved: true }).then(() => setAsk(null)); }}>
      <p>{ask?.body}</p>
    </ConfirmAction>
  </section>;
}

function Check({ ok, warn, title, detail, action }: { ok: boolean; warn?: boolean; title: string; detail: string; action?: ReactNode }) {
  const Icon = ok ? CircleCheck : warn ? CircleDashed : CircleAlert;
  return <li className="check-item"><Icon size={18} aria-hidden="true" className={ok ? 'tone-ok' : warn ? 'tone-warn' : 'tone-bad'} />
    <div><strong>{title}</strong><span className="card-hint">{detail}</span></div>{action}</li>;
}

function OverviewTab({ c, p, w }: { c: ProviderController; p: Provider; w: Workspace }) {
  const disabled = c.blocked || !!c.dialog;
  const enabled = w.keys.filter(k => k.enabled);
  const ready = enabled.filter(k => k.state === 'ready');
  const probe = [w.probe, ...w.keys.map(k => k.test)].reduce<Probe | null>((b, x) => x && (!b || x.at > b.at) ? x : b, null);
  return <div className="overview-grid">
    <section className="detail-card"><div className="card-toolbar"><h2>接入检查</h2><Button size="sm" variant="outline" disabled={disabled} onClick={() => c.setDetailTab('account')}>账户与余额</Button></div>
      <ul className="checklist">
        {!p.enabled && <Check ok={false} title="供应商已停用" detail="已停用" action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => c.toggleProvider(p)}>重新启用</Button>} />}
        <Check ok title="请求地址" detail={p.base_urls.length > 1 ? `${p.base_urls.length} 个地址轮询 · ${p.base_urls[0]}` : p.base_urls[0]} />
        <Check ok={ready.length > 0} warn={!w.keys.length} title="访问 Key" detail={w.keys.length ? `${ready.length} 个可调度 / 共 ${w.keys.length} 个` : '未配置 · 匿名'}
          action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => w.keys.length ? c.setDetailTab('keys') : c.openDialog({ kind: 'key' })}>{w.keys.length ? '管理 Key' : '添加 Key'}</Button>} />
        <Check ok={w.models.length > 0} title="模型映射" detail={w.models.length ? `已映射 ${w.models.length} 个公开模型` : '暂无模型映射'}
          action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => c.setDetailTab('models')}>{w.models.length ? '查看映射' : '映射模型'}</Button>} />
        <Check ok={!!probe && !probe.failure} warn={!probe} title="推理测试" detail={probe ? `${probe.failure ? '失败' : '成功'} · ${probe.model} · ${formatAgo(probe.at)}` : '尚未测试'}
          action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => c.openDialog({ kind: 'test' })}><Play />{probe ? '重新测试' : '测试连通'}</Button>} />
      </ul>
    </section>
    <section className="detail-card"><h2>配置</h2>
      <dl className="detail-properties">
        <dt>供应商主页</dt><dd className="break-all">{p.homepage_url ? <a className="text-link" href={p.homepage_url} target="_blank" rel="noopener noreferrer">{p.homepage_url}</a> : '未设置'}</dd>
        <dt>请求地址</dt><dd className="mono break-all">{p.base_urls.map(u => <div key={u}>{u}</div>)}</dd>
        <dt>请求渠道</dt><dd>{p.protocol ? protocolName[p.protocol] : '未提供'}</dd>
        <dt>凭据</dt><dd className="flex items-center gap-2"><KeyRound size={14} />{p.has_secret ? '已配置 · 密钥不可回显' : '未配置密钥'}</dd>
        <dt>请求超时</dt><dd>{p.timeout_seconds} 秒</dd>
        <dt>最大并发</dt><dd>{p.max_concurrent} 个请求</dd>
        <dt>配置版本</dt><dd className="mono">revision {p.revision}</dd>
      </dl>
      <h2 className="mt-5">最近测试</h2>
      {probe ? <ProbeCard probe={probe} /> : <p className="card-hint">模型推理尚未测试。</p>}
    </section>
  </div>;
}

function KeysTab({ c, w, disabled, ask }: { c: ProviderController; w: Workspace; disabled: boolean; ask: (a: Ask) => void }) {
  const f = c.features;
  return <section className="detail-card">
    <div className="card-toolbar">
      <div><h2>访问 Key</h2><p className="card-hint">Key 开关独立保存；供应商停用时，即使 Key 已启用也不参与路由。</p></div>
      <div className="toolbar-actions">
        <label className="sr-only" htmlFor="key-strategy">Key 调度策略</label>
        <select id="key-strategy" className="select-field" value={w.strategy} disabled={disabled || !w.keys.length} onChange={e => void f.run('strategy', { strategy: e.target.value })}>
          <option value="round-robin">轮流使用可用 Key</option><option value="priority">优先使用首个可用 Key（ID 顺序）</option>
        </select>
        <Button disabled={disabled || w.keys.length >= 64} onClick={() => c.openDialog({ kind: 'key' })}><Plus />添加 Key</Button>
      </div>
    </div>
    {w.keys.length ? <div className="table-scroll"><table className="data-table"><thead><tr><th scope="col">Key</th><th scope="col">默认协议 / 模型覆盖</th><th scope="col">状态</th><th scope="col">最近测试</th><th scope="col" className="text-right">操作</th></tr></thead><tbody>
      {w.keys.map(k => <tr key={k.id}>
        <td><span className="mono">{k.id}</span><KeyGroupName rate={k.rate} /></td><td>{protocolName[k.protocol as keyof typeof protocolName]}<span className="cell-hint"> · {k.model_protocols.length} 个覆盖</span></td>
        <td><span className={`health-label ${k.state}`}>{healthText[k.state] ?? k.state}</span>{k.failures > 0 && <span className="cell-hint">连续失败 {k.failures} 次</span>}</td>
        <td>{k.test ? <span className={k.test.failure ? 'text-destructive' : ''}>{k.test.failure ? '失败' : '成功'} · {k.test.elapsed_ms} ms · {formatAgo(k.test.at)}</span> : <span className="cell-hint">未测试</span>}</td>
        <td className="row-actions">
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`配置 Key ${k.id} 的模型协议`} onClick={() => { c.setDetailTab('models'); }}>模型协议</Button>
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`测试 Key ${k.id}`} onClick={() => c.openDialog({ kind: 'test', keyId: k.id })}><Play />测试</Button>
          <label className="key-toggle" data-state={k.enabled ? 'checked' : 'unchecked'}>
            <Switch className="key-toggle-switch" checked={k.enabled} disabled={disabled} aria-label={`Key ${k.id} 启用状态`} onCheckedChange={enabled => void f.run('key-put', { target: k.id, enabled })} />
            <span>{k.enabled ? '已启用' : '已停用'}</span>
          </label>
          {(k.failures > 0 || k.state === 'degraded' || k.state === 'invalid' || k.state === 'cooling' || k.state === 'half-open') && <Button size="sm" variant="ghost" disabled={disabled} aria-label={`恢复 Key ${k.id}`} onClick={() => ask({ action: 'key-reset', title: `重置 ${k.id} 的健康状态？`, body: '清除失败计数及自动调度限制；不会改变已保存密钥。', fields: { target: k.id } })}><RotateCcw />恢复</Button>}
          <Button size="sm" variant="ghost" disabled={disabled} title={`编辑 Key ${k.id}`} aria-label={`编辑 Key ${k.id}`} onClick={() => c.openDialog({ kind: 'key', keyId: k.id })}><Pencil /></Button>
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`删除 Key ${k.id}`} onClick={() => ask({ action: 'key-delete', title: `删除 Key ${k.id}？`, body: '删除后无法恢复此密钥，并会移除它对应的模型来源。其他 Key 提供的同名模型会保留；没有剩余来源的模型映射会一并删除。若模型仍是已启用 Agent 的默认模型，需先更换默认模型或停用该 Agent。', fields: { target: k.id } })}><Trash2 /></Button>
        </td>
      </tr>)}
    </tbody></table></div> : <div className="empty-state compact"><KeyRound size={24} /><p>尚无 Key。</p><Button variant="outline" disabled={disabled} onClick={() => c.openDialog({ kind: 'key' })}><Plus />添加 Key</Button></div>}
  </section>;
}

function ModelsTab({ c, w, disabled, ask }: { c: ProviderController; w: Workspace; disabled: boolean; ask: (a: Ask) => void }) {
  const selection = useMappingSelection(w);
  const batch = (operation: 'test' | 'edit' | 'delete') => c.openDialog({ kind: 'mapping-batch', operation, workspace: structuredClone(w), ids: [...selection.ids] });
  return <div className="stack">
    <section className="detail-card">
      <div className="card-toolbar"><div><h2>模型映射</h2><p className="card-hint">供应商倍率 {c.selected?.cost_multiplier}× · 相对倍率 = 供应商倍率 × Key 缓存分组倍率</p></div>
        <Button variant="outline" disabled={disabled} onClick={() => c.openDialog({ kind: 'mapping' })}><Plus />手动添加映射</Button></div>
      {w.models.length > 0 && <div className="card-toolbar" role="group" aria-label="模型映射批量操作">
        <span className="card-hint" role="status">已选 {selection.ids.length} / {w.models.length} 个映射</span>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" disabled={disabled || !selection.ids.length || c.features.needsRead} onClick={() => batch('test')}><Play />批量测试</Button>
          <Button size="sm" variant="outline" disabled={disabled || !selection.ids.length || c.features.needsRead} onClick={() => batch('edit')}><Pencil />批量编辑</Button>
          <Button size="sm" variant="outline" disabled={disabled || !selection.ids.length || c.features.needsRead} onClick={() => batch('delete')}><Trash2 />批量删除</Button>
          <Button size="sm" variant="ghost" disabled={disabled || !selection.ids.length} onClick={selection.clear}>清空选择</Button>
        </div>
      </div>}
      {w.models.length ? <div className="table-scroll"><table className="data-table"><thead><tr><th scope="col"><input type="checkbox" aria-label="全选模型映射" checked={selection.all} disabled={disabled} ref={node => { if (node) node.indeterminate = selection.ids.length > 0 && !selection.all; }} onChange={event => selection.selectAll(event.target.checked)} /></th><th scope="col">公开别名</th><th scope="col">上游模型</th><th scope="col">能力</th><th scope="col">来源 Key / 相对倍率</th><th scope="col" className="text-right">操作</th></tr></thead><tbody>
        {w.models.map(m => <tr key={m.id}>
          <td><input type="checkbox" aria-label={`选择模型映射 ${m.id}`} checked={selection.ids.includes(m.id)} disabled={disabled} onChange={event => selection.toggle(m.id, event.target.checked)} /></td>
          <td className="mono">{m.id}</td><td className="mono break-all">{m.upstream}</td>
          <td className="cell-hint">{[m.tools && '函数工具', m.streaming && '流式输出'].filter(Boolean).join(' · ') || '基础'}</td>
          <td className="cell-hint">{w.keys.filter(k => m.key_ids.length ? m.key_ids.includes(k.id) : k.enabled).map(k => <div key={k.id}><strong className="mono">{k.id}</strong><KeyMultiplierView value={k} /></div>)}{!w.keys.length && '匿名来源 · 倍率未知'}</td>
          <td className="row-actions">
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`测试模型 ${m.id}`} onClick={() => c.openDialog({ kind: 'test', model: m.upstream, keyIds: m.key_ids })}><Play />测试</Button>
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`编辑模型 ${m.id}`} onClick={() => c.openDialog({ kind: 'mapping', alias: m.id, upstream: m.upstream, tools: m.tools, streaming: m.streaming })}>编辑</Button>
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`删除模型 ${m.id}`} onClick={() => ask({ action: 'model-delete', title: `删除模型 ${m.id}？`, body: '仅移除此供应商的映射，保留其他供应商。最后一个候选被移除时删除公开模型；若 Binding 引用它则阻止删除。', fields: { target: m.id, model_revision: m.revision } })}><Trash2 /></Button>
          </td>
        </tr>)}
      </tbody></table></div> : <div className="empty-state compact"><List size={24} /><p>还没有映射模型。</p></div>}
    </section>
    <ProviderCatalogView c={c} w={w} disabled={disabled} />
  </div>;
}

function UsageTab({ c, w, disabled }: { c: ProviderController; w: Workspace; disabled: boolean }) {
  const usd = (model: string) => { const e = w.estimates.find(x => x.model === model); return e?.usd == null ? '未知 / 价格未配置' : `$${e.usd.toFixed(6)} USD`; };
  return <div className="stack">
    <section className="detail-card"><h2>用量</h2>
      {w.usage.length ? <div className="table-scroll"><table className="data-table"><thead><tr><th scope="col">模型</th><th scope="col">请求 / 失败率</th><th scope="col">平均耗时</th><th scope="col">已知 / 未知用量</th><th scope="col">输入 / 输出 token</th><th scope="col">缓存读取 / 写入</th><th scope="col">按当前价格估算</th></tr></thead><tbody>
        {w.usage.map(u => <tr key={u.model}>
          <td className="mono">{u.model}</td>
          <td>{u.requests} / {u.requests ? ((u.failures / u.requests) * 100).toFixed(1) : '0.0'}%</td>
          <td>{u.requests ? Math.round(u.elapsed_ms / u.requests) : 0} ms</td>
          <td>{u.known} / {u.requests - u.known}</td>
          <td>{u.input_tokens.toLocaleString()} / {u.output_tokens.toLocaleString()}</td>
          <td>{u.cache_read.toLocaleString()} / {u.cache_write.toLocaleString()}</td>
          <td>{usd(u.model)}</td>
        </tr>)}
      </tbody></table></div> : <div className="empty-state compact"><p>暂无用量记录。</p></div>}
    </section>
    <details className="detail-card pricing-details"><summary>费用估算与模型单价</summary><ProviderPricingView w={w} f={c.features} disabled={disabled || !w.models.length} /></details>
  </div>;
}
