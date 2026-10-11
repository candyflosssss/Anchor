import { Eye, EyeOff, Info, KeyRound, ShieldCheck } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../../components/ui/dialog';
import type { ProviderController } from './use-provider-controller';
import { protocolName, type FieldErrors } from './provider.model';
function Field({ name, title, hint, errors, children }: { name: keyof FieldErrors; title: string; hint?: string; errors: FieldErrors; children: ReactNode }) {
  return <div className="field"><label htmlFor={name}>{title}</label>{children}
    <p id={`${name}-hint`} className={errors[name] ? 'field-error' : 'field-hint'}>{errors[name] ?? hint}</p>
  </div>;
}
export function ProviderEditor({ c, restoreFocus }: { c: ProviderController; restoreFocus: () => void }) {
  const [showSecret, setShowSecret] = useState(false);
  useEffect(() => {
    const field = Object.keys(c.errors)[0];
    if (field) document.getElementById(field)?.focus();
  }, [c.errors]);
  if (!c.editor) return null;
  const { source, draft } = c.editor;
  const errorProps = (name: keyof FieldErrors) => ({ id: name, 'aria-describedby': `${name}-hint`, 'aria-invalid': !!c.errors[name] });
  return <Dialog open onOpenChange={open => { if (!open) c.closeEditor(); }}>
    <DialogContent onRequestClose={() => c.closeEditor()} onCloseAutoFocus={event => { event.preventDefault(); restoreFocus(); }}
      onInteractOutside={event => { event.preventDefault(); c.closeEditor(); }} onEscapeKeyDown={event => { event.preventDefault(); c.closeEditor(); }}>
      <header className="editor-header"><span className="eyebrow">供应商配置{c.mock ? " · 演示" : ""}</span>
        <DialogTitle className="text-xl font-semibold">{source ? `编辑 ${source.id}` : '添加供应商'}</DialogTitle>
        <DialogDescription className="mt-2 text-sm text-muted-foreground">{source ? `当前 revision ${source.revision} · 改名请使用详情中的改名操作` : '填写 API 根地址，设置初始 Key 的协议和访问凭据。'}</DialogDescription>
      </header>
      <form onSubmit={event => { event.preventDefault(); if (!c.discard) c.submit(); }} onKeyDown={event => { if (event.key === 'Enter' && event.nativeEvent.isComposing) event.preventDefault(); }} noValidate>
        <fieldset className="editor-fields" disabled={c.busy || c.problem === 'conflict'}>
          <Field name="id" title="供应商名称 / ID" hint="名称就是唯一 ID，支持中文；详情中可安全改名。" errors={c.errors}>
            <Input {...errorProps('id')} value={draft.id} onChange={event => c.changeDraft('id', event.target.value)} placeholder="例如：摸鱼AI" readOnly={!!source} autoComplete="off" />
          </Field>
          <Field name="base_url" title="API 根地址" hint="可包含 /v1；Anchor 按协议追加接口路径，不要填写完整接口地址。" errors={c.errors}>
            <Input {...errorProps('base_url')} value={draft.base_url} onChange={event => c.changeDraft('base_url', event.target.value)} placeholder="https://api.example.com/v1" autoComplete="off" spellCheck={false} />
          </Field>
          {!source && <div className="field"><label htmlFor="protocol">初始 Key 默认协议</label><select className="protocol-field" id="protocol" value={draft.protocol} onChange={event => c.changeDraft("protocol", event.target.value as typeof draft.protocol)}>{Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select><p className="field-hint">模型默认继承 Key 协议，可在 Key 的模型目录中单独覆盖。</p></div>}
          <section className="credential-section"><div className="section-label"><KeyRound size={15} /><h3>默认 API 密钥</h3><span>{source?.has_secret ? '已配置 · 不可回显' : '尚未配置'}</span></div>
            {source && <div className="secret-options" role="group" aria-label="密钥操作">{([['keep', '保留现有'], ['replace', '替换密钥'], ['clear', '清除默认密钥']] as const).map(([value, title]) => <button type="button" key={value} className={draft.secretMode === value ? 'selected' : ''} aria-pressed={draft.secretMode === value} onClick={() => { setShowSecret(false); c.changeDraft('secretMode', value); }}>{title}</button>)}</div>}
            {draft.secretMode === 'replace' && <Field name="secret" title={source ? '新的 API 密钥' : 'API 密钥（可选）'} hint="只入不出，不保存在浏览器。提交后输入即清空；失败时需重新输入。" errors={c.errors}>
              <div className="secret-input"><Input {...errorProps('secret')} type={showSecret ? 'text' : 'password'} value={draft.secret} onChange={event => c.changeDraft('secret', event.target.value)} placeholder="输入密钥，不回显已存密钥" autoComplete="new-password" spellCheck={false} />
                <button type="button" aria-label={showSecret ? '隐藏新密钥' : '显示新密钥'} onClick={() => setShowSecret(value => !value)}>{showSecret ? <EyeOff size={16} /> : <Eye size={16} />}</button></div>
            </Field>}
            {draft.secretMode === 'keep' && <p className="field-hint flex items-start gap-2"><ShieldCheck size={15} />保存时保留原密钥，不读取、不回填掩码。</p>}
            {draft.secretMode === 'clear' && <div className="warning-box"><p>清除后，依赖密钥认证的上游请求可能失败；此操作不等于停用供应商。</p><label className="check-label"><input id="secret" aria-describedby="secret-hint" aria-invalid={!!c.errors.secret} type="checkbox" checked={draft.confirmClear} onChange={event => c.changeDraft('confirmClear', event.target.checked)} />我确认清除 {source?.id} 的密钥</label><p id="secret-hint" className="field-error" role="alert">{c.errors.secret}</p></div>}
          </section>
          <div className="field-grid">
            <Field name="timeout" title="请求超时（秒）" hint="1–3600；响应头另有 30 秒上限。" errors={c.errors}><Input {...errorProps('timeout')} inputMode="numeric" value={draft.timeout} onChange={event => c.changeDraft('timeout', event.target.value)} /></Field>
            <Field name="concurrent" title="最大并发" hint="1–64；网关全局上限另为 16。" errors={c.errors}><Input {...errorProps('concurrent')} inputMode="numeric" value={draft.concurrent} onChange={event => c.changeDraft('concurrent', event.target.value)} /></Field>
          </div>
          <label className="enabled-field"><span><strong>启用此供应商</strong><small>允许被模型候选路由选用，不代表上游可用。</small></span><Switch aria-label="启用此供应商" checked={draft.enabled} onCheckedChange={value => c.changeDraft('enabled', value)} /></label>
        </fieldset>
        {c.problem && <div className="editor-problem" role="alert"><p>{c.problemMessage}</p>{c.problem === 'conflict' && <Button type="button" variant="outline" onClick={() => void c.reloadFromEditor()}>丢弃草稿并重新读取</Button>}</div>}
        {c.discard && <div className="discard-box" role="alert"><strong>有未保存的修改</strong><p>关闭将丢弃当前草稿与输入的密钥。</p><div className="flex gap-2"><Button type="button" variant="outline" onClick={c.continueEditing}>继续编辑</Button><Button type="button" variant="destructive" onClick={() => c.closeEditor(true)}>丢弃并关闭</Button></div></div>}
        <footer className="editor-footer"><span><Info size={14} />仅保存配置，不测试连通性</span><div className="flex gap-2"><Button variant="outline" type="button" disabled={c.busy} onClick={() => c.closeEditor()}>取消</Button><Button type="submit" disabled={c.busy || c.discard || c.problem === 'conflict'}>{c.busy ? '正在保存…' : source ? '保存配置' : '添加供应商'}</Button></div></footer>
      </form>
    </DialogContent>
  </Dialog>;
}
