import { Eye, EyeOff, KeyRound, Plus, X } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { ProviderCatalogView } from './provider-catalog.view';
import type { ProviderController } from './use-provider-controller';
import { MAX_PROVIDER_URLS, type FieldErrors } from './provider.model';
function Field({ name, title, errors, children }: { name: keyof FieldErrors; title: string; errors: FieldErrors; children: ReactNode }) {
  return <div className="field"><label htmlFor={name}>{title}</label>{children}
    {errors[name] && <p id={`${name}-hint`} className="field-error">{errors[name]}</p>}
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
  const errorProps = (name: keyof FieldErrors) => ({ id: name, 'aria-describedby': c.errors[name] ? `${name}-hint` : undefined, 'aria-invalid': !!c.errors[name] });
  return <Dialog open onOpenChange={open => { if (!open) c.closeEditor(); }}>
    <DialogContent className={c.editor.models ? 'provider-setup-dialog' : undefined} aria-describedby={undefined} onRequestClose={() => c.closeEditor()} onCloseAutoFocus={event => { event.preventDefault(); restoreFocus(); }}
      onInteractOutside={event => { event.preventDefault(); c.closeEditor(); }} onEscapeKeyDown={event => { event.preventDefault(); c.closeEditor(); }}>
      <header className="editor-header"><span className="eyebrow">供应商配置{c.mock ? " · 演示" : ""}</span>
        <DialogTitle className="text-xl font-semibold">{c.editor.models ? `${source?.id} · 配置模型` : source ? `编辑 ${source.id}` : '添加供应商'}</DialogTitle>
        {source && <p className="mt-2 text-sm text-muted-foreground">当前 revision {source.revision}</p>}
      </header>
      {c.editor.models ? <>
        <div className="editor-fields">
          <p className="field-hint" role="status">供应商和 Key 已保存。可在这里获取并导入模型；关闭后仍会保留已保存的配置。</p>
          {c.problem && <div role="alert" className="editor-problem">{c.problemMessage}</div>}
          {c.features.error && <div role="alert" className="editor-problem">{c.features.error}</div>}
          {c.features.data ? <ProviderCatalogView c={c} w={c.features.data} disabled={c.busy || c.blocked || c.features.needsRead} setup />
            : <p role="status">{c.features.busy ? '正在读取供应商详情…' : '未能读取供应商详情，请重新读取。'}</p>}
          <p className="field-hint">目录结果不代表推理测试通过；需要自定义别名的模型可稍后在供应商详情中配置。</p>
        </div>
        <footer className="editor-footer"><div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={c.busy} onClick={() => c.editSetup()}>修改配置 / Key</Button>
          {(c.problem || c.modelsProblem === 'conflict') && <Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.load(source?.id)}>重新读取配置</Button>}
          {(!c.features.data || c.features.error || c.features.needsRead) && <Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.features.refresh()}>重新读取详情</Button>}
          <Button type="button" disabled={c.busy} onClick={() => c.closeEditor()}>完成</Button>
        </div></footer>
      </> : <form onSubmit={event => { event.preventDefault(); setShowSecret(false); c.submit(); }} onKeyDown={event => { if (event.key === 'Enter' && event.nativeEvent.isComposing) event.preventDefault(); }} noValidate>
        <fieldset className="editor-fields" disabled={c.busy || c.problem === 'conflict'}>
          <Field name="id" title="供应商名称 / ID" errors={c.errors}>
            <Input {...errorProps('id')} value={draft.id} onChange={event => c.changeDraft('id', event.target.value)} placeholder="例如：摸鱼AI" readOnly={!!source} autoComplete="off" />
          </Field>
          <Field name="homepage_url" title="供应商主页（可选）" errors={c.errors}>
            <Input {...errorProps('homepage_url')} value={draft.homepage_url} onChange={event => c.changeDraft('homepage_url', event.target.value)} placeholder="https://www.example.com" autoComplete="off" spellCheck={false} />
          </Field>
          <div className="field"><span className="field-label" id="base_urls-label">请求地址</span>
            {draft.base_urls.map((url, i) => { const name = `base_url_${i}` as const; return <div key={i} className="url-row">
              <Input {...errorProps(name)} aria-describedby={[c.errors[name] && `${name}-hint`, c.errors.base_urls && 'base_urls-hint'].filter(Boolean).join(' ') || undefined} aria-label={i === 0 ? 'API 根地址' : `API 根地址 ${i + 1}`} value={url} onChange={event => c.changeDraft('base_urls', draft.base_urls.map((v, j) => j === i ? event.target.value : v))} placeholder="https://api.example.com/v1" autoComplete="off" spellCheck={false} />
              {draft.base_urls.length > 1 && <Button type="button" variant="outline" size="icon" aria-label={`移除请求地址 ${i + 1}`} onClick={() => c.changeDraft('base_urls', draft.base_urls.filter((_, j) => j !== i))}><X size={15} /></Button>}
              {c.errors[name] && <p id={`${name}-hint`} className="field-error">{c.errors[name]}</p>}
            </div>; })}
            {draft.base_urls.length < MAX_PROVIDER_URLS && <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => c.changeDraft('base_urls', [...draft.base_urls, ''])}><Plus size={14} />添加请求地址</Button>}
            {c.errors.base_urls && <p id="base_urls-hint" className="field-error">{c.errors.base_urls}</p>}
          </div>
          <section className="credential-section"><div className="section-label"><KeyRound size={15} /><h3>默认 API 密钥</h3><span>{source?.has_secret ? '已配置 · 不可回显' : '可选'}</span></div>
            {source && <div className="secret-options" role="group" aria-label="密钥操作">{([['keep', '保留现有'], ['replace', '替换密钥'], ['clear', '清除默认密钥']] as const).map(([value, title]) => <button type="button" key={value} className={draft.secretMode === value ? 'selected' : ''} aria-pressed={draft.secretMode === value} onClick={() => { setShowSecret(false); c.changeDraft('secretMode', value); }}>{title}</button>)}</div>}
            {(!source || draft.secretMode === 'replace') && <Field name="secret" title={source ? '新的 API 密钥' : 'API Key（可选）'} errors={c.errors}>
              <div className="secret-input"><Input {...errorProps('secret')} type={showSecret ? 'text' : 'password'} value={draft.secret} onChange={event => c.changeDraft('secret', event.target.value)} placeholder={source ? '输入密钥，不回显已存密钥' : '填写后，添加供应商并获取可用模型'} autoComplete="new-password" spellCheck={false} />
                <button type="button" aria-label={showSecret ? '隐藏新密钥' : '显示新密钥'} onClick={() => setShowSecret(value => !value)}>{showSecret ? <EyeOff size={16} /> : <Eye size={16} />}</button></div>
            </Field>}
            {!source && <p className="field-hint">填写后保存为默认 Key，并在当前页面获取、导入可用模型。留空可稍后配置。</p>}
            {source && draft.secretMode === 'clear' && <div className="warning-box"><p>清除后，依赖密钥认证的上游请求可能失败；此操作不等于停用供应商。</p><label className="check-label"><input id="secret" aria-describedby="secret-hint" aria-invalid={!!c.errors.secret} type="checkbox" checked={draft.confirmClear} onChange={event => c.changeDraft('confirmClear', event.target.checked)} />我确认清除 {source.id} 的密钥</label><p id="secret-hint" className="field-error" role="alert">{c.errors.secret}</p></div>}
          </section>
          <Field name="costMultiplier" title="供应商倍率" errors={c.errors}>
            <Input {...errorProps('costMultiplier')} type="number" min="0.000001" max="1000" step="any" value={draft.costMultiplier} onChange={event => c.changeDraft('costMultiplier', event.target.value)} />
            <p className="field-hint">默认 1。按实际支付 ÷ 到账金额填写，例如充值 1 元到账 2 元，填 0.5。最终相对倍率 = 供应商倍率 × Key 分组倍率。</p>
          </Field>
          <div className="field-grid">
            <Field name="timeout" title="请求超时（秒）" errors={c.errors}><Input {...errorProps('timeout')} inputMode="numeric" value={draft.timeout} onChange={event => c.changeDraft('timeout', event.target.value)} /></Field>
            <Field name="concurrent" title="最大并发" errors={c.errors}><Input {...errorProps('concurrent')} inputMode="numeric" value={draft.concurrent} onChange={event => c.changeDraft('concurrent', event.target.value)} /></Field>
          </div>
          <label className="enabled-field"><span><strong>启用此供应商</strong></span><Switch aria-label="启用此供应商" checked={draft.enabled} onCheckedChange={value => c.changeDraft('enabled', value)} /></label>
        </fieldset>
        {c.problem && <div className="editor-problem" role="alert"><p>{c.problemMessage}</p>{c.problem === 'conflict' && <Button type="button" variant="outline" onClick={() => void c.reloadFromEditor()}>丢弃草稿并重新读取</Button>}</div>}
        <footer className="editor-footer"><div className="flex gap-2"><Button variant="outline" type="button" disabled={c.busy} onClick={() => c.closeEditor()}>{c.editor.setup ? '完成' : '取消'}</Button><Button type="submit" disabled={c.busy || c.problem === 'conflict'}>{c.busy ? '正在保存…' : c.editor.setup ? '保存并获取模型' : source ? '保存配置' : draft.secret.trim() ? '添加并获取模型' : '添加供应商'}</Button></div></footer>
      </form>}
    </DialogContent>
  </Dialog>;
}
