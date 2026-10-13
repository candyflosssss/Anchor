import { isValidModelAlias } from './provider-features.model';
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { CircleAlert, Play } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { ProbeCard } from './provider-detail.view';
import { protocolName } from './provider.model';
import { isValidId, type ProviderDialog } from './provider-summary.model';
import type { Probe } from './provider-features.model';
import type { ProviderController } from './use-provider-controller';

/** Every provider sub-action dialog. The dialog itself is the explicit confirmation step (approved: true). */
export function ProviderDialogs({ c }: { c: ProviderController }) {
  const d = c.dialog; const p = c.selected; const w = c.features.data;
  if (!d || !p || !w) return null;
  return <Shell c={c} d={d} />;
}

const titles: Record<ProviderDialog['kind'], string> = { test: '测试连通', rename: '改名', copy: '复制为新供应商', delete: '删除供应商', key: 'Key', mapping: '模型映射' };

function Shell({ c, d }: { c: ProviderController; d: ProviderDialog }) {
  const [tried, setTried] = useState(false);
  const f = c.features;
  const close = () => { if (!f.busy) c.closeDialog(); };
  const run = async (action: string, fields: Parameters<typeof f.run>[1], keepOpen = false) => {
    setTried(true);
    const ok = await f.run(action, { ...fields, approved: true });
    if (ok && !keepOpen) c.closeDialog();
    return ok;
  };
  const body = d.kind === 'test' ? <TestBody c={c} d={d} run={run} />
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

function TestBody({ c, d, run }: { c: ProviderController; d: Extract<ProviderDialog, { kind: 'test' }>; run: Run }) {
  const w = c.features.data!;
  const [model, setModel] = useState(d.model ?? w.models[0]?.upstream ?? '');
  const [key, setKey] = useState(d.keyId ?? w.keys.find(k => k.enabled)?.id ?? w.keys[0]?.id ?? 'anonymous');
  const options = [...new Set([...w.models.map(m => m.upstream), ...(c.models?.models ?? [])])];
  async function submit(e: FormEvent) {
    e.preventDefault(); if (!model.trim()) return;
    await run('test', { model: model.trim(), target: key }, true);
  }
  // Show only a probe produced while this dialog is open, for the model currently entered.
  const [opened] = useState(() => Date.now() / 1000 - 1);
  const result: Probe | null = w.probe && w.probe.at >= opened && w.probe.model === model.trim() ? w.probe : null;
  return <form onSubmit={submit} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="test-model">上游模型 ID</label>
        <Input id="test-model" list="test-model-options" value={model} onChange={e => setModel(e.target.value)} placeholder="例如 gpt-4.1-mini" autoComplete="off" />
        <datalist id="test-model-options">{options.map(id => <option key={id} value={id} />)}</datalist></div>
      <div className="field"><label htmlFor="test-key">使用 Key</label>
        <select id="test-key" className="select-field w-full" value={key} onChange={e => setKey(e.target.value)}>
          {w.keys.length ? w.keys.map(k => <option value={k.id} key={k.id}>{k.id}{k.enabled ? '' : '（已停用，仍可显式测试）'}</option>) : <option value="anonymous">匿名</option>}
        </select></div>
      <p className="warning-box">将发送一次推理请求，可能产生少量用量。</p>
      {result && <div aria-live="polite"><ProbeCard probe={result} /></div>}
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !model.trim()}><Play />{c.features.busy ? '正在测试…' : result ? '再测一次' : '发起测试'}</Button></Footer>
  </form>;
}

function RenameBody({ c, run }: { c: ProviderController; run: Run }) {
  const p = c.selected!;
  const [id, setId] = useState(p.id);
  const error = id === p.id ? '' : !isValidId(id) ? '1–128 个中文／字母／数字或 ._-，以字母或数字开头。' : c.providers.some(x => x.id === id) ? '已存在同名供应商。' : '';
  return <form onSubmit={e => { e.preventDefault(); if (!error && id !== p.id) void run('rename', { target: id }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="rename-id">新名称</label><Input id="rename-id" value={id} onChange={e => setId(e.target.value.trim())} aria-invalid={!!error} aria-describedby={error ? 'rename-hint' : undefined} autoComplete="off" />
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
      <div className="field"><label htmlFor="copy-id">新供应商名称</label><Input id="copy-id" value={id} onChange={e => setId(e.target.value.trim())} aria-invalid={!!error} aria-describedby={error ? 'copy-hint' : undefined} autoComplete="off" />
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
  const idError = existing ? '' : !isValidId(id) ? '1–128 个字母／数字或 ._-，以字母或数字开头。' : w.keys.some(k => k.id === id) ? '已存在同名 Key；如需更换请使用「编辑」。' : '';
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
      <div className="field"><label htmlFor="key-id">Key 名称</label><Input id="key-id" value={id} readOnly={!!existing} onChange={e => setId(e.target.value.trim())} aria-invalid={!!idError} aria-describedby={idError ? 'key-id-hint' : undefined} autoComplete="off" />
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
  const [multiplier, setMultiplier] = useState(String(editing?.multiplier ?? 1));
  const [strategy, setStrategy] = useState(editing?.strategy ?? 'round-robin');
  const multiplierValue = Number(multiplier);
  const multiplierError = !multiplier.trim() || !Number.isFinite(multiplierValue) || multiplierValue < 0.01 || multiplierValue > 1000 ? '倍率须为 0.01–1000。' : '';
  const [tools, setTools] = useState(editing?.tools ?? d.tools ?? true);
  const [streaming, setStreaming] = useState(editing?.streaming ?? d.streaming ?? true);
  const aliasError = !alias ? '' : !isValidModelAlias(alias) ? '1–128 个字母／数字或 ._-，以字母或数字开头。' : !editing && w.models.some(m => m.id === alias) ? '此供应商已有同名映射；请改用「编辑」。' : '';
  const options = c.models?.models ?? [];
  return <form onSubmit={e => { e.preventDefault(); if (alias && upstream.trim() && !aliasError && !multiplierError) void run('model-put', { target: alias, model: upstream.trim(), multiplier: multiplierValue, strategy, tools, streaming, model_revision: editing?.revision ?? 0 }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="mapping-upstream">上游模型 ID</label>
        <Input id="mapping-upstream" list="mapping-upstream-options" value={upstream} autoComplete="off" onChange={e => { setUpstream(e.target.value); if (!touched) setAlias(e.target.value.trim()); }} />
        <datalist id="mapping-upstream-options">{options.map(id => <option key={id} value={id} />)}</datalist></div>
      <div className="field"><label htmlFor="mapping-alias">Anchor 公开别名</label>
        <Input id="mapping-alias" value={alias} readOnly={!!editing} placeholder="例如 coding-main" autoComplete="off" aria-invalid={!!aliasError} aria-describedby={aliasError ? 'mapping-alias-hint' : undefined} onChange={e => { setAlias(e.target.value.trim()); setTouched(true); }} />
        {aliasError && <p id="mapping-alias-hint" className="field-error">{aliasError}</p>}</div>
      <div className="field"><label htmlFor="mapping-multiplier">此供应商的相对倍率</label>
        <Input id="mapping-multiplier" type="number" min="0.01" max="1000" step="any" value={multiplier} onChange={e => setMultiplier(e.target.value)} aria-invalid={!!multiplierError} />
        {multiplierError && <p className="field-error">{multiplierError}</p>}</div>
      {editing && <div className="field"><label htmlFor="mapping-strategy">公开模型调度策略</label>
        <select id="mapping-strategy" className="select-field" value={strategy} onChange={e => setStrategy(e.target.value)}><option value="round-robin">按倍率加权轮询</option><option value="priority">按候选顺序优先</option></select></div>}
      <label className="checkbox-row"><input type="checkbox" checked={tools} onChange={e => setTools(e.target.checked)} />函数工具</label>
      <label className="checkbox-row"><input type="checkbox" checked={streaming} onChange={e => setStreaming(e.target.checked)} />流式输出</label>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !alias || !upstream.trim() || !!aliasError || !!multiplierError}>{editing ? '保存映射' : '添加映射'}</Button></Footer>
  </form>;
}
