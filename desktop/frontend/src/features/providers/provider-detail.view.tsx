import { useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { ArrowLeft, CircleAlert, CircleCheck, CircleDashed, Copy, Download, KeyRound, List, LoaderCircle, MoreHorizontal, Pencil, Play, Plus, Power, RotateCcw, ShieldCheck, Trash2, TextCursorInput } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { StatusBadge } from '../../components/shared/status-badge';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { MenuItem, Popover } from '../../components/shared/popover-menu';
import { ProviderPricingView } from './provider-pricing.view';
import { healthText, type Action, type Probe, type Workspace } from './provider-features.model';
import { catalogFailureText, catalogProblemText, protocolName, type Provider } from './provider.model';
import { formatAgo, type DetailTab, type ProviderSummary, type Tone } from './provider-summary.model';
import type { ProviderController } from './use-provider-controller';

const tabs: [DetailTab, string][] = [['overview', '概况'], ['keys', 'Key'], ['models', '模型'], ['usage', '用量']];
type Ask = { action: string; title: string; body: string; fields: Partial<Action> };

export function HealthPill({ tone, label }: { tone: Tone; label: string }) {
  return <span className={`health-pill tone-${tone}`}><span className="mini-dot" aria-hidden="true" />{label}</span>;
}

export function ProbeCard({ probe }: { probe: Probe }) {
  return <div className={`probe-result ${probe.failure ? 'failed' : 'passed'}`}>
    <strong>{probe.failure ? '推理测试失败' : '推理测试成功'}</strong>
    <span> · HTTP {probe.http_status || '—'} · {probe.elapsed_ms} ms · Key {probe.key_id || '匿名'}</span>
    <div className="mono break-all">{probe.model} · revision {probe.revision} · {formatAgo(probe.at)}</div>
    {probe.failure && <div>失败原因：{probe.failure}</div>}
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
  return <section className="provider-page" aria-label={`${p.id} 的详情`} aria-busy={f.busy || c.modelsBusy}>
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
        <Button variant="outline" disabled={blocked} onClick={() => c.requestToggle(p)}><Power />{p.enabled ? '停用供应商' : '启用供应商'}</Button>
        <Popover menu label={`${p.id} 的更多操作`} disabled={blocked || !w} trigger={props => <Button variant="ghost" size="icon" aria-label="更多操作" {...props}><MoreHorizontal /></Button>}>
          <MenuItem onSelect={() => c.openDialog({ kind: 'copy' })}><Copy size={14} />复制为新供应商</MenuItem>
          <MenuItem onSelect={() => c.openDialog({ kind: 'rename' })}><TextCursorInput size={14} />改名</MenuItem>
          <MenuItem destructive onSelect={() => c.openDialog({ kind: 'delete' })}><Trash2 size={14} />删除供应商</MenuItem>
        </Popover>
      </div>
    </header>
    <div className="detail-tabs" role="tablist" aria-label="供应商详情分区" onKeyDown={tabKeys}>
      {tabs.map(([id, title]) => <button key={id} ref={node => { tabRefs.current[id] = node; }} role="tab" id={`tab-${id}`} aria-controls={`panel-${id}`} aria-selected={c.detailTab === id} tabIndex={c.detailTab === id ? 0 : -1} onClick={() => c.setDetailTab(id)}>
        {title}{id === 'keys' && w ? <span className="tab-count">{w.keys.length}</span> : id === 'models' && w ? <span className="tab-count">{w.models.length}</span> : null}
      </button>)}
    </div>
    <div role="tabpanel" id={`panel-${c.detailTab}`} aria-labelledby={`tab-${c.detailTab}`} className="detail-panel" tabIndex={0}>
      {f.error && !c.dialog && <div className="feedback error-feedback" role="alert"><CircleAlert size={16} /><span>{f.error}</span><Button variant="outline" size="sm" disabled={f.busy} onClick={() => void (f.needsRead ? c.load() : f.refresh())}>重新读取</Button></div>}
      {!w ? (!f.error && <div className="empty-state compact" role="status"><LoaderCircle className="loading-icon" size={24} /><p>正在读取 Key、模型与用量…</p></div>)
        : c.detailTab === 'overview' ? <OverviewTab c={c} p={p} w={w} />
        : c.detailTab === 'keys' ? <KeysTab c={c} w={w} disabled={blocked} ask={setAsk} />
        : c.detailTab === 'models' ? <ModelsTab c={c} w={w} disabled={blocked} ask={setAsk} />
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
    <section className="detail-card"><h2>接入检查</h2><p className="card-hint">按顺序完成后即可被 Agent 调用。</p>
      <ul className="checklist">
        {!p.enabled && <Check ok={false} title="供应商已停用" detail="不参与新的路由选择；配置和密钥均保留。" action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => c.requestToggle(p)}>重新启用</Button>} />}
        <Check ok title="请求地址" detail={p.base_urls.length > 1 ? `${p.base_urls.length} 个地址轮询 · ${p.base_urls[0]}` : p.base_urls[0]} />
        <Check ok={ready.length > 0} warn={!w.keys.length} title="访问 Key" detail={w.keys.length ? `${ready.length} 个可调度 / 共 ${w.keys.length} 个` : '未配置，将以匿名方式请求上游'}
          action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => w.keys.length ? c.setDetailTab('keys') : c.openDialog({ kind: 'key' })}>{w.keys.length ? '管理 Key' : '添加 Key'}</Button>} />
        <Check ok={w.models.length > 0} title="模型映射" detail={w.models.length ? `已映射 ${w.models.length} 个公开模型` : 'Agent 还不能通过此供应商使用任何模型'}
          action={<Button size="sm" variant="outline" disabled={disabled} onClick={() => c.setDetailTab('models')}>{w.models.length ? '查看映射' : '映射模型'}</Button>} />
        <Check ok={!!probe && !probe.failure} warn={!probe} title="推理测试" detail={probe ? `${probe.failure ? '失败' : '成功'} · ${probe.model} · ${formatAgo(probe.at)}` : '尚未测试；可调度只代表配置完整'}
          action={<Button size="sm" variant={probe && !probe.failure ? 'outline' : 'default'} disabled={disabled} onClick={() => c.openDialog({ kind: 'test' })}><Play />{probe ? '重新测试' : '测试连通'}</Button>} />
      </ul>
    </section>
    <section className="detail-card"><h2>配置</h2>
      <dl className="detail-properties">
        <dt>供应商主页</dt><dd className="break-all">{p.homepage_url ? <a className="text-link" href={p.homepage_url} target="_blank" rel="noopener noreferrer">{p.homepage_url}</a> : '未设置'}</dd>
        <dt>请求地址</dt><dd className="mono break-all">{p.base_urls.map(u => <div key={u}>{u}</div>)}</dd>
        <dt>凭据</dt><dd className="flex items-center gap-2"><KeyRound size={14} />{p.has_secret ? '已配置 · 密钥不可回显' : '未配置密钥'}</dd>
        <dt>请求超时</dt><dd>{p.timeout_seconds} 秒</dd>
        <dt>最大并发</dt><dd>{p.max_concurrent} 个请求</dd>
        <dt>配置版本</dt><dd className="mono">revision {p.revision}</dd>
      </dl>
      <p className="detail-footnote"><ShieldCheck size={14} />停用不会删除配置或已存密钥。</p>
      <h2 className="mt-5">最近测试</h2>
      {probe ? <ProbeCard probe={probe} /> : <p className="card-hint">模型推理尚未测试。</p>}
    </section>
  </div>;
}

function KeysTab({ c, w, disabled, ask }: { c: ProviderController; w: Workspace; disabled: boolean; ask: (a: Ask) => void }) {
  const f = c.features;
  return <section className="detail-card">
    <div className="card-toolbar">
      <div><h2>访问 Key</h2><p className="card-hint">秘密只入不出；提交即清空。最多 64 个 Key。</p></div>
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
        <td className="mono">{k.id}</td><td>{protocolName[k.protocol as keyof typeof protocolName]}<span className="cell-hint"> · {k.model_protocols.length} 个覆盖</span></td>
        <td><span className={`health-label ${k.state}`}>{healthText[k.state] ?? k.state}</span>{k.failures > 0 && <span className="cell-hint">连续失败 {k.failures} 次</span>}</td>
        <td>{k.test ? <span className={k.test.failure ? 'text-destructive' : ''}>{k.test.failure ? '失败' : '成功'} · {k.test.elapsed_ms} ms · {formatAgo(k.test.at)}</span> : <span className="cell-hint">未测试</span>}</td>
        <td className="row-actions">
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`配置 Key ${k.id} 的模型协议`} onClick={() => { c.selectCatalogKey(k.id); c.setDetailTab('models'); }}>模型协议</Button>
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`测试 Key ${k.id}`} onClick={() => c.openDialog({ kind: 'test', keyId: k.id })}><Play />测试</Button>
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`编辑 Key ${k.id}`} onClick={() => c.openDialog({ kind: 'key', keyId: k.id })}>编辑</Button>
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`${k.enabled ? '停用' : '启用'} Key ${k.id}`} onClick={() => void f.run('key-put', { target: k.id, enabled: !k.enabled })}>{k.enabled ? '停用' : '启用'}</Button>
          {(k.failures > 0 || k.state === 'invalid' || k.state === 'cooling' || k.state === 'half-open') && <Button size="sm" variant="ghost" disabled={disabled} aria-label={`恢复 Key ${k.id}`} onClick={() => ask({ action: 'key-reset', title: `恢复 ${k.id} 的调度？`, body: '清除认证失效、失败计数及冷却；不会改变已保存密钥。', fields: { target: k.id } })}><RotateCcw />恢复</Button>}
          <Button size="sm" variant="ghost" disabled={disabled} aria-label={`删除 Key ${k.id}`} onClick={() => ask({ action: 'key-delete', title: `删除 Key ${k.id}？`, body: '删除后无法回显或恢复此密钥。最后一个 Key 被删除后会使用匿名请求。', fields: { target: k.id } })}><Trash2 /></Button>
        </td>
      </tr>)}
    </tbody></table></div> : <div className="empty-state compact"><KeyRound size={24} /><p>尚无 Key，网关将尝试匿名请求。</p><Button variant="outline" disabled={disabled} onClick={() => c.openDialog({ kind: 'key' })}><Plus />添加 Key</Button></div>}
    <p className="detail-footnote">认证失败自动跳过；限流按 Retry-After 冷却。连续 3 次故障熔断 30 秒。临时状态在修改配置或重启核心后重置。</p>
  </section>;
}

function ModelsTab({ c, w, disabled, ask }: { c: ProviderController; w: Workspace; disabled: boolean; ask: (a: Ask) => void }) {
  const models = c.models;
  const key = w.keys.find(k => k.id === c.catalogKey) ?? (!c.catalogKey ? w.keys.find(k => k.enabled) : undefined);
  const modelIDs = [...new Set([...(models?.models ?? []), ...(key?.model_protocols.map(m => m.model) ?? [])])];
  const mapped = new Map(w.models.map(m => [m.upstream, m.id]));
  return <div className="stack">
    <section className="detail-card">
      <div className="card-toolbar"><div><h2>模型映射</h2><p className="card-hint">Agent 通过公开别名调用，Anchor 转发到此供应商的上游模型。</p></div>
        <Button variant="outline" disabled={disabled} onClick={() => c.openDialog({ kind: 'mapping' })}><Plus />手动添加映射</Button></div>
      {w.models.length ? <div className="table-scroll"><table className="data-table"><thead><tr><th scope="col">公开别名</th><th scope="col">上游模型</th><th scope="col">能力</th><th scope="col">调度 / 倍率</th><th scope="col" className="text-right">操作</th></tr></thead><tbody>
        {w.models.map(m => <tr key={m.id}>
          <td className="mono">{m.id}</td><td className="mono break-all">{m.upstream}</td>
          <td className="cell-hint">{[m.tools && '函数工具', m.streaming && '流式输出'].filter(Boolean).join(' · ') || '基础'}</td>
          <td className="cell-hint">{m.strategy === 'round-robin' ? '加权轮询' : '顺序优先'} · {m.multiplier}× · {m.candidate_count} 个候选</td>
          <td className="row-actions">
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`测试模型 ${m.id}`} onClick={() => c.openDialog({ kind: 'test', model: m.upstream })}><Play />测试</Button>
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`编辑模型 ${m.id}`} onClick={() => c.openDialog({ kind: 'mapping', alias: m.id, upstream: m.upstream, tools: m.tools, streaming: m.streaming })}>编辑</Button>
            <Button size="sm" variant="ghost" disabled={disabled} aria-label={`删除模型 ${m.id}`} onClick={() => ask({ action: 'model-delete', title: `删除模型 ${m.id}？`, body: '仅移除此供应商的映射，保留其他供应商。最后一个候选被移除时删除公开模型；若 Binding 引用它则阻止删除。', fields: { target: m.id, model_revision: m.revision } })}><Trash2 /></Button>
          </td>
        </tr>)}
      </tbody></table></div> : <div className="empty-state compact"><List size={24} /><p>还没有映射模型。先从上游目录导入，或手动添加。</p></div>}
    </section>
    <section className="detail-card model-directory" aria-label="上游模型目录" aria-busy={c.modelsBusy}>
      <div className="card-toolbar"><div><h2>Key 的模型与协议</h2><p className="card-hint">使用所选 Key 读取站点模型目录；每个实际模型默认继承该 Key 的协议。</p></div>
        <div className="toolbar-actions"><label htmlFor="catalog-key">查询 Key</label><select id="catalog-key" className="select-field" value={c.catalogKey} disabled={disabled} onChange={e => c.selectCatalogKey(e.target.value)}>
          <option value="">{w.keys.length ? '首个启用的 Key' : '匿名请求'}</option>{w.keys.map(k => <option key={k.id} value={k.id}>{k.id}{k.enabled ? '' : '（已停用）'}</option>)}
        </select><Button variant="outline" disabled={disabled || (!!c.catalogKey && !key)} onClick={() => void c.discoverModels()}>{c.modelsBusy ? <LoaderCircle className="loading-icon" /> : <Download />}{c.modelsBusy ? '正在获取目录…' : '获取模型目录'}</Button></div></div>
      {key && <p className="card-hint">Key {key.id} · 默认协议 {protocolName[key.protocol as keyof typeof protocolName]}；更改默认协议会影响所有继承的模型。</p>}
      {c.modelsProblem && <p className="catalog-feedback" role="alert">{catalogProblemText(c.modelsProblem)}</p>}
      <p className="catalog-feedback" aria-live="polite">{models ? `${models.failure ? '目录获取失败' : `已获取 ${models.models.length} 个模型`} · Key ${models.key_id || '—'} · HTTP ${models.http_status || '—'} · ${models.elapsed_ms} ms · revision ${models.revision}` : ''}</p>
      {models?.failure && <p className="catalog-feedback" role="alert">{catalogFailureText[models.failure]}</p>}
      {models && !models.failure && models.models.length === 0 && <p className="catalog-feedback">上游返回空目录；未推断其推理能力。</p>}
      {modelIDs.length > 0 && <><p className="detail-footnote">协议覆盖仅作用于此 Key 和实际上游模型名。目录成功不代表推理可用；已保存的覆盖始终保留在列表中。</p>
        <ul className="catalog-list" tabIndex={0} aria-label="上游模型 ID">{modelIDs.map(id => <li key={id}><span className="mono break-all">{id}</span>
          {models && !models.failure && <span className="cell-hint">{models.models.includes(id) ? '目录列出' : '仅已存协议'}</span>}
          {key && <select className="select-field" aria-label={`${id} 的协议`} value={key.model_protocols.find(m => m.model === id)?.protocol ?? 'inherit'} disabled={disabled || c.features.needsRead} onChange={e => void c.features.run('key-model-protocol', { target: key.id, model: id, protocol: e.target.value })}>
            <option value="inherit">继承 Key（{protocolName[key.protocol as keyof typeof protocolName]}）</option>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>}
          {mapped.has(id) ? <span className="cell-hint">已映射为 {mapped.get(id)}</span> : <Button size="sm" variant="ghost" disabled={disabled} aria-label={`导入 ${id}`} onClick={() => c.openDialog({ kind: 'mapping', upstream: id })}><Plus />导入</Button>}</li>)}</ul></>}
    </section>
  </div>;
}

function UsageTab({ c, w, disabled }: { c: ProviderController; w: Workspace; disabled: boolean }) {
  const usd = (model: string) => { const e = w.estimates.find(x => x.model === model); return e?.usd == null ? '未知 / 价格未配置' : `$${e.usd.toFixed(6)} USD`; };
  return <div className="stack">
    <section className="detail-card"><h2>用量</h2>
      <p className="card-hint">统计每次上游尝试（包括重试和测试）。仅累计已返回的 token；未知用量单独显示。缓存输入包含在总输入中。</p>
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
