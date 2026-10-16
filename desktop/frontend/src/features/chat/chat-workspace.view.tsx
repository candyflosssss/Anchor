import { useEffect, useMemo, useRef, useState } from 'react';
import { Check, Copy, LoaderCircle, MessageSquare, RefreshCw, Send, Square, Trash2 } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { ConfirmAction } from '../../components/shared/confirm-action';
import { prototype, type ProviderEnvironment } from '../../desktop/environment';
import { chatProtocol, chatReasoningLevels, DesktopChatTransport, type ChatTransport } from './chat.model';
import { MockChatTransport } from './chat.mock';
import { useChat, useChatOptions } from './use-chat';

const selectClass = 'h-9 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50';

export function ChatWorkspace({ environment }: { environment: ProviderEnvironment }) {
  const options = useChatOptions(environment.transport);
  const transport = useMemo(() => prototype ? new MockChatTransport() : new DesktopChatTransport(), []);
  const [busy, setBusy] = useState(false);
  const revision = options.workspace?.revision ?? 0;
  const ready = !options.loading && !!options.workspace && options.workspace.provider_id === options.providerId;
  return <main className="flex min-w-0 flex-1 flex-col gap-4 p-4 md:p-6" aria-label="对话测试工作区">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div><h1 className="text-xl font-semibold">对话测试</h1><p className="mt-1 text-sm text-muted-foreground">选择供应商、Key 和模型，快速测试真实的多轮对话。{environment.mock && '（Mock 预览，不会请求上游）'}</p></div>
      <Button variant="outline" size="sm" disabled={busy || options.loading} onClick={options.refresh}><RefreshCw className="mr-2 size-4" />刷新配置</Button>
    </div>
    <section className="rounded-lg border p-4" aria-label="对话目标">
      <div className="grid min-w-0 gap-3 md:grid-cols-3">
        <label className="min-w-0 space-y-1.5 text-sm"><span>供应商</span><select aria-label="对话供应商" className={selectClass} disabled={busy || options.loading} value={options.providerId} onChange={e => options.setProviderId(e.target.value)}>
          {!options.providers.length && <option value="">{options.loading ? '正在读取…' : '暂无启用的供应商'}</option>}
          {options.providers.map(p => <option key={p.id} value={p.id}>{p.id}</option>)}
        </select></label>
        <label className="min-w-0 space-y-1.5 text-sm"><span>Key</span><select aria-label="对话 Key" className={selectClass} disabled={busy || !ready} value={options.keyId} onChange={e => options.setKeyId(e.target.value)}>
          {!options.keyId && <option value="">{options.loading ? '正在读取…' : '暂无启用的 Key'}</option>}
          {ready && (options.workspace!.keys.length ? options.workspace!.keys.filter(k => k.enabled).map(k => <option key={k.id} value={k.id}>{k.id} · {k.protocol}</option>) : <option value="anonymous">anonymous（无密钥）</option>)}
        </select></label>
        <label className="min-w-0 space-y-1.5 text-sm"><span>模型</span><div className="flex min-w-0 gap-2"><Input aria-label="对话模型" list="chat-model-options" placeholder="选择或输入上游模型 ID" disabled={busy || !ready || !options.keyId} value={options.model} maxLength={256} onChange={e => options.setModel(e.target.value)} />
          <Button variant="outline" size="icon" aria-label="刷新对话模型目录" disabled={busy || !ready || !options.keyId || options.catalogLoading} onClick={options.refreshCatalog}>{options.catalogLoading ? <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" /> : <RefreshCw className="size-4" />}</Button></div>
          <datalist id="chat-model-options">{options.models.map(m => <option key={m} value={m} />)}</datalist>
        </label>
      </div>
      <p className="mt-3 text-xs text-muted-foreground">只显示 Key 标识，不显示密钥。切换目标会开始新对话；模型使用上游 ID，不经过公开模型的候选调度。</p>
      {options.catalogError && <p role="status" className="mt-2 text-sm text-muted-foreground">{options.catalogError}</p>}
      {options.error && <p role="alert" className="mt-2 text-sm text-destructive">{options.error}</p>}
      {!options.loading && !options.error && !options.providers.length && <p className="mt-2 text-sm text-muted-foreground">请先在“供应商”页面添加并启用供应商。</p>}
    </section>
    <Conversation key={`${options.providerId}/${revision}/${options.keyId}/${options.model}`} transport={transport}
      providerId={options.providerId} revision={revision} keyId={options.keyId} model={options.model} protocol={chatProtocol(options.workspace?.keys.find(k => k.id === options.keyId), options.model)}
      ready={ready && !!options.keyId && !!options.model.trim()} onBusy={setBusy} />
  </main>;
}

function Conversation({ transport, providerId, revision, keyId, model, protocol, ready, onBusy }: {
  transport: ChatTransport; providerId: string; revision: number; keyId: string; model: string; protocol: string; ready: boolean; onBusy: (busy: boolean) => void;
}) {
  const chat = useChat(transport);
  const [confirmClear, setConfirmClear] = useState(false);
  const [copied, setCopied] = useState(-1);
  const [copyError, setCopyError] = useState('');
  const scroll = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const composer = useRef<HTMLTextAreaElement>(null);
  const composing = useRef(false);
  const wasBusy = useRef(false);
  useEffect(() => { onBusy(chat.busy); return () => onBusy(false); }, [chat.busy, onBusy]);
  useEffect(() => { if (scroll.current && follow.current) scroll.current.scrollTop = scroll.current.scrollHeight; }, [chat.turns]);
  useEffect(() => {
    if (wasBusy.current && !chat.busy && ready) composer.current?.focus({ preventScroll: true });
    wasBusy.current = chat.busy;
  }, [chat.busy, ready]);
  async function copy(text: string, index: number) {
    setCopyError('');
    try { await navigator.clipboard.writeText(text); setCopied(index); }
    catch { setCopyError('复制失败，请手动选择回复文本复制。'); }
  }
  function send() { follow.current = true; setCopied(-1); void chat.send(providerId, revision, keyId, model); }
  return <section className="flex min-h-[420px] min-w-0 flex-1 flex-col overflow-hidden rounded-lg border" aria-label="测试对话">
    <div className="space-y-2 border-b px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3"><span className="text-sm font-medium">当前对话</span><div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm"><span className="shrink-0">思考深度</span><select aria-label="思考深度" aria-describedby="chat-effort-help" className={selectClass} disabled={chat.busy || !ready} value={chat.reasoningEffort} onChange={e => chat.setReasoningEffort(e.target.value)}>
          <option value="">默认（不指定）</option>{chatReasoningLevels(protocol).map(level => <option key={level.value} value={level.value}>{level.label}</option>)}
        </select></label>
        <Button variant="ghost" size="sm" disabled={chat.busy || !chat.turns.length} onClick={() => setConfirmClear(true)}><Trash2 className="mr-2 size-4" />清空对话</Button>
      </div></div>
      <p id="chat-effort-help" className="text-xs text-muted-foreground">调整深度不清空对话，下一轮生效。支持的档位由上游模型决定，较高深度可能增加耗时和费用。{protocol === 'anthropic' && ' Anthropic 使用原生 effort，不额外开启 thinking。'}</p>
    </div>
    <div ref={scroll} role="log" aria-label="对话消息" aria-live="off" className="min-h-[220px] max-h-[55vh] flex-1 space-y-6 overflow-y-auto p-4 md:p-6" onScroll={() => { const el = scroll.current; if (el) follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80; }}>
      {!chat.turns.length && <div className="flex min-h-[200px] flex-col items-center justify-center gap-3 text-center text-muted-foreground"><MessageSquare className="size-8" /><p className="text-sm">{ready ? '发送一条消息，开始测试对话' : '先选择供应商、Key 和模型'}</p><p className="text-xs">仅保留当前页面会话，不保存聊天记录</p></div>}
      {chat.turns.map((turn, index) => <div key={index} className="space-y-4">
        <article aria-label="你的消息" className="ml-auto max-w-[90%] rounded-lg bg-muted px-4 py-3 md:max-w-[80%]"><p className="mb-2 text-xs font-medium text-muted-foreground">你</p><p className="whitespace-pre-wrap break-words text-sm leading-7 [overflow-wrap:anywhere]">{turn.content}</p></article>
        <article aria-label="助手回复" className="max-w-full"><div className="mb-2 flex items-center gap-2"><span className="text-xs font-medium text-muted-foreground">助手</span>{turn.reply && <Button variant="ghost" size="icon" className="size-7" aria-label={`复制第 ${index + 1} 条回复`} onClick={() => void copy(turn.reply, index)}>{copied === index ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}</Button>}</div>
          <p className="whitespace-pre-wrap break-words text-sm leading-7 [overflow-wrap:anywhere]">{turn.reply || (turn.status === 'running' ? '正在等待模型回复…' : turn.status === 'cancelled' ? '已停止生成' : '未收到回复')}</p>
          {turn.status === 'cancelled' && turn.reply && <p className="mt-2 text-xs text-muted-foreground">已停止 · 未完成回复不计入后续上下文</p>}
          {turn.status === 'failed' && <p className="mt-2 text-xs text-destructive">生成失败 · 此轮不计入后续上下文</p>}
        </article>
      </div>)}
    </div>
    <div className="border-t p-4">
      {(chat.error || copyError) && <p role="alert" className="mb-3 text-sm text-destructive">{chat.error || copyError}</p>}
      <form onSubmit={e => { e.preventDefault(); send(); }} className="space-y-3">
        <textarea ref={composer} aria-label="对话消息输入" placeholder="输入消息…" value={chat.draft} disabled={chat.busy || !ready} rows={3} maxLength={65536}
          className="flex w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          onChange={e => chat.setDraft(e.target.value)} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={() => { composing.current = false; }}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !composing.current && !e.nativeEvent.isComposing && e.nativeEvent.keyCode !== 229) { e.preventDefault(); if (!chat.busy && ready) send(); } }} />
        <div className="flex flex-wrap items-center justify-between gap-3"><div className="space-y-1 text-xs text-muted-foreground"><p>Enter 发送 · Shift+Enter 换行</p><p>发送即请求所选上游，可能产生费用。离开页面会尝试停止生成。</p><p role="status" aria-live="polite">{chat.busy ? (chat.stopping ? '正在停止…' : '正在生成…') : chat.turns.length ? '本轮已结束' : '尚未发送'}{chat.elapsed > 0 && ` · ${(chat.elapsed / 1000).toFixed(1)} 秒`}</p></div>
          {chat.busy ? <Button type="button" variant="outline" onClick={() => void chat.stop()} disabled={chat.stopping}><Square className="mr-2 size-4" />{chat.stopping ? '正在停止…' : '停止生成'}</Button> : <Button type="submit" disabled={!ready || !chat.draft.trim()}><Send className="mr-2 size-4" />发送</Button>}
        </div>
      </form>
    </div>
    <ConfirmAction open={confirmClear} busy={false} onCancel={() => setConfirmClear(false)} title="清空当前对话？" onConfirm={() => { chat.clear(); setConfirmClear(false); setCopied(-1); setCopyError(''); }}>此操作只清除页面中的消息，不会删除请求日志。对话清空后无法恢复。</ConfirmAction>
  </section>;
}
