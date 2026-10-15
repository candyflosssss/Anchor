import { routingRules } from '../models/model-routing.model';
import { isValidModelAlias } from './provider-features.model';
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { CircleAlert, Play } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { ProbeCard } from './provider-detail.view';
import { protocolName } from './provider.model';
import { isValidId, type ProviderDialog } from './provider-summary.model';
import { useProviderTest } from './use-provider-test';
import { keyForMappingTest } from './provider-test.model';
import type { ProviderController } from './use-provider-controller';

/** Every provider sub-action dialog. The dialog itself is the explicit confirmation step (approved: true). */
export function ProviderDialogs({ c }: { c: ProviderController }) {
  const d = c.dialog; const p = c.selected;
  if (!d || !p) return null;
  return <Shell key={`${p.id}:${d.kind}`} c={c} d={d} />;
}

const titles: Record<ProviderDialog['kind'], string> = { test: '测试连通', rename: '改名', copy: '复制为新供应商', delete: '删除供应商', key: 'Key', mapping: '模型映射' };

function Shell({ c, d }: { c: ProviderController; d: ProviderDialog }) {
  const [tried, setTried] = useState(false);
  const f = c.features;
  const close = () => { if (!f.busy || !f.data) c.closeDialog(); };
  const run = async (action: string, fields: Parameters<typeof f.run>[1], keepOpen = false) => {
    setTried(true);
    const ok = await f.run(action, { ...fields, approved: true });
    if (ok && !keepOpen) c.closeDialog();
    return ok;
  };
  const body = !f.data ? <div className="editor-fields"><p role={f.error ? 'alert' : 'status'}>{f.error || '正在读取供应商 Key…'}</p>{f.error && <Button type="button" variant="outline" onClick={() => void f.refresh()}>重新读取</Button>}</div>
    : d.kind === 'test'  ? <TestBody c={c} d={d} />
    : d.kind === 'rename' ? <RenameBody c={c} run={run} />
    : d.kind === 'copy' ? <CopyBody c={c} run={run} />
    : d.kind === 'delete' ? <DeleteBody c={c} run={run} />
    : d.kind === 'key' ? <KeyBody c={c} d={d} run={run} />
    : <MappingBody c={c} d={d} run={run} />;
  return <Dialog open onOpenChange={open => { if (!open) close(); }}>
    <DialogContent aria-describedby={undefined} className="action-dialog" closeLabel="关闭对话框" onRequestClose={close}
      onInteractOutside={e => { e.preventDefault(); close(); }} onEscapeKeyDown={e => { e.preventDefault(); close(); }}>
      <header className="editor-header"><span className="eyebrow">{c.selected?.id}{c.mock ? ' · 演示' : ''}</span>
        <DialogTitle className="text-xl font-semibold">{d.kind === 'key' ? (d.keyId ? `编辑 Key ${d.keyId}` : '添加 Key') : d.kind === 'mapping' ? (d.alias ? `编辑模型 ${d.alias}` : '添加模型映射') : titles[d.kind]}</DialogTitle>
      </header>
      {tried && f.error && <p className="dialog-error" role="alert"><CircleAlert size={15} />{f.error}</p>}
      {tried && f.notice && <p className="feedback notice-feedback" role="status">{f.notice}</p>}
      {body}
    </DialogContent>
  </Dialog>;
}
type Run = (action: string, fields: Parameters<ProviderController['features']['run']>[1], keepOpen?: boolean) => Promise<boolean>;

function Footer({ c, children }: { c: ProviderController; children: ReactNode }) {
  return <footer className="editor-footer">
    <div className="flex gap-2"><Button type="button" variant="outline" disabled={c.features.busy} onClick={c.closeDialog}>取消</Button>{children}</div></footer>;
}

function TestBody({ c, d }: { c: ProviderController; d: Extract<ProviderDialog, { kind: 'test' }> }) {
  const w = c.features.data!;
  const keys = w.keys.filter(k => k.enabled);
  const configured = !!d.model;
  const [selectedKey, setKey] = useState(keys.some(k => k.id === d.keyId) ? d.keyId! : keys[0]?.id ?? (w.keys.length ? '' : 'anonymous'));
  const key = configured ? keyForMappingTest(w, d.keyIds ?? []) : selectedKey;
  const test = useProviderTest(c.selected!, key, d.model ?? '', c.transport, c.features.refresh);
  const output = useRef<HTMLPreElement | null>(null);
  useEffect(() => { if (output.current) output.current.scrollTop = output.current.scrollHeight; }, [test.progress?.text]);
  return <form onSubmit={e => { e.preventDefault(); void test.start(); }} noValidate>
    <fieldset className="editor-fields" disabled={test.busy || c.features.busy}>
      {configured ? <div className="stack">
        <div className="field"><span>上游模型</span><p className="mono break-all">{d.model}</p></div>
        <div className="field"><span>使用 Key</span><p>{key === 'anonymous' ? '匿名（无 Key）' : key || '没有可用的 Key'}</p></div>
        {!key && <p role="alert" className="field-error">{d.keyIds?.length ? '此模型映射绑定的 Key 均未启用或已不存在，请先检查映射与 Key 配置。' : '此供应商没有启用的 Key，请先启用 Key。'}</p>}
      </div> : <>
        <div className="field"><label htmlFor="test-key">使用 Key</label>
          <select id="test-key" className="select-field w-full" value={key} onChange={e => setKey(e.target.value)}>
            {w.keys.length ? keys.length ? keys.map(k => <option value={k.id} key={k.id}>{k.id}</option>) : <option value="">没有启用的 Key</option> : <option value="anonymous">匿名（无 Key）</option>}
          </select></div>
        <div className="field"><label htmlFor="test-model">可用模型</label>
          <select id="test-model" className="select-field w-full" value={test.model} disabled={test.loading || !test.models.length} onChange={e => test.setModel(e.target.value)}>
            {!test.models.length && <option value="">{test.loading ? '正在读取此 Key 的可用模型…' : '没有可用模型'}</option>}
            {test.models.map(id => <option key={id} value={id}>{id}</option>)}
          </select></div>
        {test.catalogError && <div role="alert" className="field-error">{test.catalogError}<Button type="button" size="sm" variant="outline" onClick={test.refreshCatalog}>重新获取模型</Button></div>}
      </>}
      <p className="warning-copy">将发送一次推理请求，可能产生少量用量。</p>
    </fieldset>
    {(test.busy || test.progress || test.error) && <section className="test-live-output" aria-label="实时返回信息">
      <p role="status">{test.busy ? test.progress?.phase === 'receiving' ? '正在接收返回…' : '正在连接上游…' : test.progress?.status === 'cancelled' ? '测试已停止' : test.error ? '测试中断' : '测试完成'}{test.progress ? ` · ${test.progress.elapsed_ms} ms` : ''}</p>
      <pre ref={output} tabIndex={0} aria-label="模型返回内容">{test.progress?.text || (test.busy ? '等待模型返回…' : '未收到文本内容。')}</pre>
      {test.progress?.result && <ProbeCard probe={test.progress.result} />}
      {test.error && <p className="field-error" role="alert">{test.error}</p>}
    </section>}
    <footer className="editor-footer"><div className="flex gap-2">
      <Button type="button" variant="outline" onClick={c.closeDialog}>{test.busy ? '关闭并停止' : '关闭'}</Button>
      {test.busy ? <Button type="button" onClick={() => void test.stop()}>停止测试</Button> : <Button disabled={c.features.busy || test.loading || !test.model || !key || !c.transport.probe}><Play size={14} />发起测试</Button>}
    </div></footer>
  </form>;
}

function RenameBody({ c, run }: { c: ProviderController; run: Run }) {
  const p = c.selected!;
  const [id, setId] = useState(p.id);
  const error = id === p.id ? '' : !isValidId(id) ? '1–128 个中文／字母／数字或 ._-，以字母或数字开头。' : c.providers.some(x => x.id === id) ? '已存在同名供应商。' : '';
  return <form onSubmit={e => { e.preventDefault(); if (!error && id !== p.id) void run('rename', { target: id }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="rename-id">新名称</label><Input id="rename-id" value={id} onChange={e => setId(e.target.value)} aria-invalid={!!error} aria-describedby={error ? 'rename-hint' : undefined} autoComplete="off" />
        {error && <p id="rename-hint" className="field-error">{error}</p>}</div>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !!error || id === p.id}>确认改名</Button></Footer>
  </form>;
}

function CopyBody({ c, run }: { c: ProviderController; run: Run }) {
  const p = c.selected!;
  const [id, setId] = useState(() => { let n = `${p.id}-copy`; for (let i = 2; c.providers.some(x => x.id === n); i++) n = `${p.id}-copy${i}`; return n; });
  const [keys, setKeys] = useState(false);
  const error = !isValidId(id) ? '1–128 个中文／字母／数字或 ._-，以字母或数字开头。' : c.providers.some(x => x.id === id) ? '已存在同名供应商。' : '';
  return <form onSubmit={e => { e.preventDefault(); if (!error) void run('copy', { target: id, copy_keys: keys }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="copy-id">新供应商名称</label><Input id="copy-id" value={id} onChange={e => setId(e.target.value)} aria-invalid={!!error} aria-describedby={error ? 'copy-hint' : undefined} autoComplete="off" />
        {error && <p id="copy-hint" className="field-error">{error}</p>}</div>
      <label className="checkbox-row"><input type="checkbox" checked={keys} onChange={e => setKeys(e.target.checked)} />同时复制 Key</label>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !!error}>确认复制</Button></Footer>
  </form>;
}

function DeleteBody({ c, run }: { c: ProviderController; run: Run }) {
  const p = c.selected!; const w = c.features.data!;
  return <form onSubmit={e => { e.preventDefault(); if (!w.models.length) void run('delete', {}); }}>
    <div className="editor-fields">
      {w.models.length ? <div className="warning-box">仍有 {w.models.length} 个模型映射到 {p.id}。请先删除这些映射，再删除供应商。
        <div className="mt-2"><Button type="button" size="sm" variant="outline" onClick={() => { c.closeDialog(); c.setDetailTab('models'); }}>查看模型映射</Button></div></div>
        : <p>确认删除 <strong className="break-all">{p.id}</strong>？{w.keys.length ? `${w.keys.length} 个 Key 将一并删除且无法恢复。` : ''}</p>}
    </div>
    <Footer c={c}><Button variant="destructive" disabled={c.features.busy || !!w.models.length}>{c.features.busy ? '正在删除…' : '确认删除'}</Button></Footer>
  </form>;
}

function KeyBody({ c, d, run }: { c: ProviderController; d: Extract<ProviderDialog, { kind: 'key' }>; run: Run }) {
  const w = c.features.data!;
  const existing = d.keyId ? w.keys.find(k => k.id === d.keyId) : undefined;
  const [id, setId] = useState(() => { if (d.keyId) return d.keyId; let i = w.keys.length + 1; while (w.keys.some(k => k.id === `key-${i}`)) i++; return `key-${i}`; });
  const [secret, setSecret] = useState('');
  const secretRef = useRef(''); secretRef.current = secret;
  useEffect(() => () => { secretRef.current = ''; }, []);
  const idError = existing ? '' : !isValidId(id) ? '1–128 个中文／字母／数字或 ._-，以字母或数字开头。' : w.keys.some(k => k.id === id) ? '已存在同名 Key；如需更换请使用「编辑」。' : '';
  const [protocol, setProtocol] = useState(existing?.protocol ?? 'responses');
  const trimmed = secret.trim();
  const secretError = /[\r\n]/.test(trimmed) ? '密钥不能包含换行。' : new TextEncoder().encode(trimmed).length > 8192 ? '密钥不能超过 8192 字节。' : '';
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (idError || secretError || (!existing && !trimmed)) return;
    const value = trimmed; setSecret('');
    await run('key-put', { target: id, secret: value, protocol, enabled: existing?.enabled ?? true });
  }
  return <form onSubmit={submit} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="key-id">Key 名称</label><Input id="key-id" value={id} readOnly={!!existing} onChange={e => setId(e.target.value)} aria-invalid={!!idError} aria-describedby={idError ? 'key-id-hint' : undefined} autoComplete="off" />
        {idError && <p id="key-id-hint" className="field-error">{idError}</p>}</div>
      <div className="field"><label htmlFor="key-secret">{existing ? '新的 API Key（留空保留）' : 'API Key'}</label><Input id="key-secret" type="password" autoComplete="new-password" spellCheck={false} value={secret} onChange={e => setSecret(e.target.value)} aria-invalid={!!secretError} aria-describedby={secretError ? 'key-secret-hint' : undefined} />
        {secretError && <p id="key-secret-hint" className="field-error">{secretError}</p>}</div>
      <div className="field"><label htmlFor="key-protocol">Key 默认协议</label><select id="key-protocol" className="select-field" value={protocol} onChange={e => setProtocol(e.target.value)}>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !!idError || !!secretError || (!existing && !trimmed)}>保存 Key</Button></Footer>
  </form>;
}

function MappingBody({ c, d, run }: { c: ProviderController; d: Extract<ProviderDialog, { kind: 'mapping' }>; run: Run }) {
  const w = c.features.data!;
  const editing = d.alias ? w.models.find(m => m.id === d.alias) : undefined;
  const [upstream, setUpstream] = useState(d.upstream ?? '');
  const [alias, setAlias] = useState(d.alias ?? d.upstream ?? '');
  const [touched, setTouched] = useState(!!d.alias);
  const [strategy, setStrategy] = useState(editing?.strategy ?? 'round-robin');
  const [keyIDs, setKeyIDs] = useState<string[]>(editing?.key_ids ?? c.models?.entries.find(e => e.id === d.upstream)?.key_ids ?? []);
  const [tools, setTools] = useState(editing?.tools ?? d.tools ?? true);
  const [streaming, setStreaming] = useState(editing?.streaming ?? d.streaming ?? true);
  const aliasError = !alias ? '' : !isValidModelAlias(alias) ? '1–128 个中文／字母／数字或 ._-，以字母或数字开头。' : !editing && w.models.some(m => m.id === alias) ? '此供应商已有同名映射；请改用「编辑」。' : '';
  const options = c.models?.models ?? [];
  return <form onSubmit={e => { e.preventDefault(); if (alias && upstream.trim() && !aliasError) void run('model-put', { target: alias, model: upstream.trim(), key_ids: keyIDs, strategy, tools, streaming, model_revision: editing?.revision ?? 0 }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="mapping-upstream">上游模型 ID</label>
        <Input id="mapping-upstream" list="mapping-upstream-options" value={upstream} autoComplete="off" onChange={e => { setUpstream(e.target.value); const source = c.models?.entries.find(v => v.id === e.target.value.trim()); setKeyIDs(source?.key_ids ?? []); if (!touched) setAlias(e.target.value); }} />
        <datalist id="mapping-upstream-options">{options.map(id => <option key={id} value={id} />)}</datalist></div>
      <div className="field"><label htmlFor="mapping-alias">Anchor 公开别名</label>
        <Input id="mapping-alias" value={alias} readOnly={!!editing} placeholder="例如 coding-main" autoComplete="off" aria-invalid={!!aliasError} aria-describedby={aliasError ? 'mapping-alias-hint' : undefined} onChange={e => { setAlias(e.target.value); setTouched(true); }} />
        {aliasError && <p id="mapping-alias-hint" className="field-error">{aliasError}</p>}</div>
      <p className="field-hint">相对倍率自动取「供应商倍率 × 来源 Key 的缓存分组倍率」。模型倍率暂不启用。</p>
      {editing && <div className="field"><label htmlFor="mapping-strategy">公开模型调度策略</label>
        <select id="mapping-strategy" className="select-field" value={strategy} onChange={e => setStrategy(e.target.value)}>{Object.entries(routingRules).map(([id, rule]) => <option key={id} value={id}>{rule.label}</option>)}</select></div>}
      <div className="field"><label>来源 Key（ID）</label><p className="field-hint">不勾选表示允许所有启用 Key；导入时默认保留目录来源。</p>
        {w.keys.map(k => <label key={k.id} className="checkbox-row"><input type="checkbox" checked={keyIDs.includes(k.id)} onChange={e => setKeyIDs(ids => e.target.checked ? [...ids, k.id] : ids.filter(id => id !== k.id))} />{k.id}{!k.enabled && ' · 已停用'}</label>)}
        {keyIDs.includes('anonymous') && <label className="checkbox-row"><input type="checkbox" checked onChange={() => setKeyIDs([])} />anonymous · 匿名来源</label>}
      </div>
      <label className="checkbox-row"><input type="checkbox" checked={tools} onChange={e => setTools(e.target.checked)} />函数工具</label>
      <label className="checkbox-row"><input type="checkbox" checked={streaming} onChange={e => setStreaming(e.target.checked)} />流式输出</label>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !alias || !upstream.trim() || !!aliasError}>{editing ? '保存映射' : '添加映射'}</Button></Footer>
  </form>;
}
