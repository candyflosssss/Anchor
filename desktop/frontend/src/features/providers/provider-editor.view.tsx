import { Eye, EyeOff, KeyRound, LoaderCircle, Plus, X } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { Dialog, DialogContent, DialogTitle } from '../../components/ui/dialog';
import { ProviderInitialKeys } from './provider-initial-keys.view';
import { ProviderGroupRates } from './provider-group-rates.view';
import { ProviderCatalogView } from './provider-catalog.view';
import type { ProviderController } from './use-provider-controller';
import { protocolName, MAX_PROVIDER_URLS, type FieldErrors } from './provider.model';
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
    <DialogContent className={c.editor.models ? 'provider-editor-dialog provider-setup-dialog' : 'provider-editor-dialog'} aria-describedby={undefined} onRequestClose={() => c.closeEditor()} onCloseAutoFocus={event => { event.preventDefault(); restoreFocus(); }}
      onInteractOutside={event => { event.preventDefault(); c.closeEditor(); }} onEscapeKeyDown={event => { event.preventDefault(); c.closeEditor(); }}>
      <header className="editor-header">
        <DialogTitle className="text-xl font-semibold">{c.editor.models ? `${source?.id} · 配置模型` : source ? `编辑 ${source.id}` : '添加供应商'}{c.mock && <span className="ml-2 text-sm font-normal text-muted-foreground"> · 演示</span>}</DialogTitle>
      </header>
      {c.editor.models ? <>
        <div className="editor-fields">
          {c.problem && <div role="alert" className="editor-problem">{c.problemMessage}</div>}
          {c.features.error && <div role="alert" className="editor-problem">{c.features.error}</div>}
          <ProviderGroupRates key={`${source?.id}:${source?.revision}`} c={c} />
          {c.features.data ? <ProviderCatalogView c={c} w={c.features.data} disabled={c.busy || c.blocked || c.features.needsRead} setup />
            : !c.features.error && <div className="provider-setup-state" role="status">{c.busy && <LoaderCircle className="loading-icon" aria-hidden="true" />}{c.busy ? '加载中…' : '读取失败'}</div>}
        </div>
        <footer className="editor-footer"><div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={c.busy} onClick={() => c.editSetup()}>修改配置 / Key</Button>
          {(c.problem || c.modelsProblem === 'conflict') && <Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.load(source?.id)}>重新读取配置</Button>}
          {!c.busy && (!c.features.data || c.features.error || c.features.needsRead) && <Button type="button" variant="outline" disabled={c.busy} onClick={() => void c.features.refresh()}>重新读取详情</Button>}
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
          <Field name="protocol" title="请求渠道" errors={c.errors}>
            <select {...errorProps('protocol')} aria-describedby={c.errors.protocol ? 'protocol-hint protocol-scope' : 'protocol-scope'} className="select-field" value={draft.protocol} onChange={event => c.changeDraft('protocol', event.target.value)}>
              {source && !source.protocol && <option value="">保留现有渠道</option>}
              {Object.entries(protocolName).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
            <p id="protocol-scope" className="field-hint">更改后用于匿名请求和默认 Key；其他 Key 及模型单独设置的协议保持不变。</p>
          </Field>
          <section className="credential-section"><div className="section-label"><KeyRound size={15} /><h3>{source ? '默认 API 密钥' : 'API Keys'}</h3><span>{source?.has_secret ? '已有 Key' : '可选'}</span></div>
            {source && <div className="secret-options" role="group" aria-label="密钥操作">{([['keep', '保留现有'], ['replace', '替换密钥'], ['clear', '清除默认密钥']] as const).map(([value, title]) => <button type="button" key={value} className={draft.secretMode === value ? 'selected' : ''} aria-pressed={draft.secretMode === value} onClick={() => { setShowSecret(false); c.changeDraft('secretMode', value); }}>{title}</button>)}</div>}
            {(!source || draft.secretMode === 'replace') && <Field name="secret" title={source ? '新的 API 密钥' : 'API Key（可选）'} errors={c.errors}>
              <div className="secret-input"><Input {...errorProps('secret')} type={showSecret ? 'text' : 'password'} value={draft.secret} onChange={event => c.changeDraft('secret', event.target.value)} placeholder={source ? '新的 API Key' : 'API Key'} autoComplete="new-password" spellCheck={false} />
                <button type="button" aria-label={showSecret ? '隐藏新密钥' : '显示新密钥'} onClick={() => setShowSecret(value => !value)}>{showSecret ? <EyeOff size={16} /> : <Eye size={16} />}</button></div>
            </Field>}
            {source && draft.secretMode === 'clear' && <div className="warning-box"><p>清除默认密钥会同步移除它对应的模型来源；其他 Key 的同名模型保留，没有剩余来源的模型映射一并删除。若模型仍是已启用 Agent 的默认模型，需先更换默认模型或停用该 Agent。</p><label className="check-label"><input id="secret" aria-describedby="secret-hint" aria-invalid={!!c.errors.secret} type="checkbox" checked={draft.confirmClear} onChange={event => c.changeDraft('confirmClear', event.target.checked)} />我确认清除 {source.id} 的密钥</label><p id="secret-hint" className="field-error" role="alert">{c.errors.secret}</p></div>}
            {!source && <ProviderInitialKeys draft={draft} errors={c.errors} onChange={keys => c.changeDraft('initialKeys', keys)} />}
          </section>
          <Field name="costMultiplier" title="供应商倍率" errors={c.errors}>
            <Input {...errorProps('costMultiplier')} type="number" min="0.000001" max="1000" step="any" value={draft.costMultiplier} onChange={event => c.changeDraft('costMultiplier', event.target.value)} />
          </Field>
          <div className="field-grid">
            <Field name="timeout" title="请求超时（秒）" errors={c.errors}><Input {...errorProps('timeout')} inputMode="numeric" value={draft.timeout} onChange={event => c.changeDraft('timeout', event.target.value)} /></Field>
            <Field name="concurrent" title="最大并发" errors={c.errors}><Input {...errorProps('concurrent')} inputMode="numeric" value={draft.concurrent} onChange={event => c.changeDraft('concurrent', event.target.value)} /></Field>
          </div>
          <label className="enabled-field"><span><strong>启用此供应商</strong></span><Switch aria-label="启用此供应商" checked={draft.enabled} onCheckedChange={value => c.changeDraft('enabled', value)} /></label>
        </fieldset>
        {c.problem && <div className="editor-problem" role="alert"><p>{c.problemMessage}</p>{c.problem === 'conflict' && <Button type="button" variant="outline" onClick={() => void c.reloadFromEditor()}>丢弃草稿并重新读取</Button>}</div>}
        <footer className="editor-footer"><div className="flex gap-2"><Button variant="outline" type="button" disabled={c.busy} onClick={() => c.closeEditor()}>{c.editor.setup ? '完成' : '取消'}</Button><Button type="submit" disabled={c.busy || c.problem === 'conflict'}>{c.busy ? '正在保存…' : c.editor.setup ? '保存并获取模型' : source ? '保存配置' : (draft.secret.trim() || draft.initialKeys.length) ? '添加并获取模型' : '添加供应商'}</Button></div></footer>
      </form>}
    </DialogContent>
  </Dialog>;
}
