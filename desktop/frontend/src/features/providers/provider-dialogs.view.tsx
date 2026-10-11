import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { CircleAlert, Play } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
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
    <DialogContent className="action-dialog" closeLabel="关闭对话框" onRequestClose={close}
      onInteractOutside={e => { e.preventDefault(); close(); }} onEscapeKeyDown={e => { e.preventDefault(); close(); }}>
      <header className="editor-header"><span className="eyebrow">{c.selected?.id}{c.mock ? ' · 演示' : ''}</span>
        <DialogTitle className="text-xl font-semibold">{d.kind === 'key' ? (d.keyId ? `编辑 Key ${d.keyId}` : '添加 Key') : d.kind === 'mapping' ? (d.alias ? `编辑模型 ${d.alias}` : '添加模型映射') : titles[d.kind]}</DialogTitle>
        <DialogDescription className="mt-2 text-sm text-muted-foreground">{descriptions[d.kind]}</DialogDescription>
      </header>
      {tried && f.error && <p className="dialog-error" role="alert"><CircleAlert size={15} />{f.error}</p>}
      {body}
    </DialogContent>
  </Dialog>;
}
const descriptions: Record<ProviderDialog['kind'], string> = {
  test: '发送一次最小推理请求，确认上游真的能生成内容。',
  rename: '模型候选与密钥会在同一事务中迁移；在途请求保留旧快照。',
  copy: '新供应商默认停用，确认无误后再启用。',
  delete: '将删除配置和所有密钥。存在模型引用时会阻止删除；历史用量保留。',
  key: '秘密只入不出；提交即清空。最多 64 个 Key。',
  mapping: 'Agent 使用公开别名调用；Anchor 转发到这里填写的上游模型。',
};
type Run = (action: string, fields: Parameters<ProviderController['features']['run']>[1], keepOpen?: boolean) => Promise<boolean>;

function Footer({ c, children }: { c: ProviderController; children: ReactNode }) {
  return <footer className="editor-footer"><span>{c.mock ? '本次为模拟操作，不会修改真实实例。' : '将修改本机实例的真实配置。'}</span>
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
        <datalist id="test-model-options">{options.map(id => <option key={id} value={id} />)}</datalist>
        {!options.length && <p className="field-hint">尚无映射；可先到「模型」获取上游目录。</p>}</div>
      <div className="field"><label htmlFor="test-key">使用 Key</label>
        <select id="test-key" className="select-field w-full" value={key} onChange={e => setKey(e.target.value)}>
          {w.keys.length ? w.keys.map(k => <option value={k.id} key={k.id}>{k.id}{k.enabled ? '' : '（已停用，仍可显式测试）'}</option>) : <option value="anonymous">匿名</option>}
        </select></div>
      <p className="warning-box">将发送一次最小推理请求，可能产生少量用量。测试会绕过冷却供诊断，不修改启停设置。</p>
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
      <div className="field"><label htmlFor="rename-id">新名称</label><Input id="rename-id" value={id} onChange={e => setId(e.target.value.trim())} aria-invalid={!!error} aria-describedby="rename-hint" autoComplete="off" />
        <p id="rename-hint" className={error ? 'field-error' : 'field-hint'}>{error || '名称同时作为 CLI 中的供应商 ID。'}</p></div>
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
      <div className="field"><label htmlFor="copy-id">新供应商名称</label><Input id="copy-id" value={id} onChange={e => setId(e.target.value.trim())} aria-invalid={!!error} aria-describedby="copy-hint" autoComplete="off" />
        <p id="copy-hint" className={error ? 'field-error' : 'field-hint'}>{error || (keys ? '将复制配置和所有密钥，新供应商默认停用。' : '仅复制配置，新供应商默认停用，不含密钥和模型映射。')}</p></div>
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
      <div className="field"><label htmlFor="key-id">Key 名称</label><Input id="key-id" value={id} readOnly={!!existing} onChange={e => setId(e.target.value.trim())} aria-invalid={!!idError} aria-describedby="key-id-hint" autoComplete="off" />
        <p id="key-id-hint" className={idError ? 'field-error' : 'field-hint'}>{idError || '仅用于区分和日志显示，不是密钥本身。'}</p></div>
      <div className="field"><label htmlFor="key-secret">{existing ? '新的 API Key' : 'API Key'}</label><Input id="key-secret" type="password" autoComplete="new-password" spellCheck={false} value={secret} onChange={e => setSecret(e.target.value)} aria-invalid={!!secretError} aria-describedby="key-secret-hint" />
        <p id="key-secret-hint" className={secretError ? 'field-error' : 'field-hint'}>{secretError || (existing ? '留空保留现有密钥；提交后输入框立即清空。' : '保存后无法回显；提交后输入框立即清空。')}</p></div>
      <div className="field"><label htmlFor="key-protocol">Key 默认协议</label><select id="key-protocol" className="select-field" value={protocol} onChange={e => setProtocol(e.target.value)}>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select><p className="field-hint">此 Key 下未单独设置协议的模型继承这个值。</p></div>
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
  const [tools, setTools] = useState(d.tools ?? false);
  const [streaming, setStreaming] = useState(d.streaming ?? false);
  const aliasError = !alias ? '' : !isValidId(alias) ? '1–128 个字母／数字或 ._-，以字母或数字开头。' : !editing && w.models.some(m => m.id === alias) ? '此供应商已有同名映射；请改用「编辑」。' : '';
  const options = c.models?.models ?? [];
  return <form onSubmit={e => { e.preventDefault(); if (alias && upstream.trim() && !aliasError) void run('model-put', { target: alias, model: upstream.trim(), tools, streaming, model_revision: editing?.revision ?? 0 }); }} noValidate>
    <fieldset className="editor-fields" disabled={c.features.busy}>
      <div className="field"><label htmlFor="mapping-upstream">上游模型 ID</label>
        <Input id="mapping-upstream" list="mapping-upstream-options" value={upstream} autoComplete="off" onChange={e => { setUpstream(e.target.value); if (!touched) setAlias(e.target.value.trim()); }} />
        <datalist id="mapping-upstream-options">{options.map(id => <option key={id} value={id} />)}</datalist></div>
      <div className="field"><label htmlFor="mapping-alias">Anchor 公开别名</label>
        <Input id="mapping-alias" value={alias} readOnly={!!editing} placeholder="例如 coding-main" autoComplete="off" aria-invalid={!!aliasError} aria-describedby="mapping-alias-hint" onChange={e => { setAlias(e.target.value.trim()); setTouched(true); }} />
        <p id="mapping-alias-hint" className={aliasError ? 'field-error' : 'field-hint'}>{aliasError || 'Agent 请求时填写的模型名；多个供应商可映射同一别名互为候选。'}</p></div>
      <label className="checkbox-row"><input type="checkbox" checked={tools} onChange={e => setTools(e.target.checked)} />函数工具</label>
      <label className="checkbox-row"><input type="checkbox" checked={streaming} onChange={e => setStreaming(e.target.checked)} />流式输出</label>
    </fieldset>
    <Footer c={c}><Button disabled={c.features.busy || !alias || !upstream.trim() || !!aliasError}>{editing ? '保存映射' : '添加映射'}</Button></Footer>
  </form>;
}
