import { useRef } from 'react';
import { ArrowRight, Check, CircleAlert, Copy, CornerDownRight, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '../../components/ui/sheet';
import { sourceLabels, statusLabels, type RequestLog } from './log.model';

export function RequestDetail({ row, onClose, onCopy, notice }: { notice: string; row: RequestLog | null; onClose: () => void; onCopy: (row: RequestLog) => void }) {
  const opener = useRef<HTMLElement | null>(null);
  return <Sheet open={!!row} onOpenChange={open => { if (!open) onClose(); }}>
    <SheetContent className="log-detail-sheet w-full max-w-[620px] p-0 pt-5 sm:max-w-[620px]" closeLabel="关闭请求详情"
      onOpenAutoFocus={() => { opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; }}
      onCloseAutoFocus={event => { event.preventDefault(); opener.current?.focus(); }}>
      {row && <>
        <SheetHeader className="border-b px-6 pb-5 pt-2 text-left">
          <div className="flex items-center gap-2 text-xs text-muted-foreground"><span>{sourceLabels[row.source]}</span><span>·</span><span>虚拟数据</span></div>
          <SheetTitle className="pr-8 text-xl">请求详情</SheetTitle>
          <SheetDescription className="break-all font-mono text-xs">{row.id}</SheetDescription>
          <div className="flex items-center justify-between gap-3 pt-2"><span className={`log-result log-result-${row.status}`}><span className="mini-dot" />{statusLabels[row.status]}{row.status === 'failed' && row.http === 200 ? ' · HTTP 200' : ''}</span><Button variant="outline" size="sm" onClick={() => onCopy(row)}><Copy size={13} />复制排障摘要</Button></div>
        </SheetHeader>
        <div className="log-detail-body">
          {notice && <p role="status" className="text-xs text-muted-foreground">{notice}</p>}
          <section className={`log-explanation ${row.status === 'failed' ? 'is-failed' : ''}`}>
            <div className="flex gap-3">{row.status === 'succeeded' ? <Check size={18} className="mt-0.5 shrink-0" /> : <CircleAlert size={18} className="mt-0.5 shrink-0" />}<div><h3 className="text-sm font-medium">{row.summary}</h3><p className="mt-2 text-xs leading-6 text-muted-foreground">{row.status === 'failed' && row.http === 200 ? 'HTTP 200 表示上游成功返回。Anchor 的协议校验、转换与最终交付需要单独判断。' : row.delivery}</p></div></div>
          </section>
          <section aria-label="处理结果"><h3 className="log-section-title">处理结果</h3><div className="log-outcome-grid"><div><span>上游传输</span><strong>{row.http === null ? '未收到 HTTP 状态' : `HTTP ${row.http}`}</strong></div><ArrowRight size={14} /><div><span>Anchor 处理</span><strong>{statusLabels[row.status]}</strong></div><ArrowRight size={14} /><div><span>客户端交付</span><strong>{row.delivery}</strong></div></div></section>
          <section><h3 className="log-section-title">路由与配置快照</h3><dl className="log-properties">
            <div><dt>公开模型</dt><dd className="mono">{row.model}</dd></div><div><dt>供应商 / Key</dt><dd>{row.provider} / <span className="mono">{row.key}</span></dd></div>
            <div><dt>协议路径</dt><dd>{row.protocol}</dd></div><div><dt>配置版本</dt><dd className="mono">revision {row.revision}</dd></div>
            <div><dt>发生时间</dt><dd className="mono">2026-10-11 {row.time} +08:00</dd></div><div><dt>总耗时</dt><dd className="mono">{row.elapsed === null ? '未知' : `${row.elapsed.toLocaleString()} ms`}</dd></div>
          </dl></section>
          <section aria-label="尝试时间线"><div className="mb-4 flex items-center justify-between"><h3 className="log-section-title mb-0">尝试时间线</h3><span className="text-xs text-muted-foreground">{row.attempts.length} 次上游尝试</span></div>
            {row.attempts.length === 0 ? <p className="rounded-md border border-dashed p-4 text-xs text-muted-foreground">请求在路由阶段结束，没有调用上游。</p> : <ol className="log-timeline">{row.attempts.map(item => <li key={item.number}><span className={`log-attempt-dot ${item.outcome === '成功' || item.outcome === '协议通过' ? 'is-success' : ''}`}>{item.number}</span><div className="log-attempt-card"><header><strong>{item.provider}<span className="ml-2 text-xs font-normal text-muted-foreground">{item.key}</span></strong><span className="mono text-xs text-muted-foreground">{row.status === 'interrupted' ? '耗时未知' : `${item.elapsed.toLocaleString()} ms`}</span></header><p className="mono mt-2 break-all text-xs text-muted-foreground">{item.model} <span className="mx-1">·</span> {item.protocol}</p><div className="mt-3 flex items-center gap-2 text-xs"><span className="protocol-tag">{item.http === null ? 'HTTP 未知' : `HTTP ${item.http}`}</span><span className="font-medium">{item.outcome}</span></div><p className="mt-2 text-xs leading-6 text-muted-foreground">{item.note}</p>{item.code && <p className="mono mt-1 text-xs text-destructive">{item.code}</p>}</div>{item.number < row.attempts.length && <p className="log-retry"><CornerDownRight size={12} />输出前切换候选</p>}</li>)}</ol>}
          </section>
          <section><h3 className="log-section-title">用量</h3><div className="log-usage"><div><span>输入 Token</span><strong>{row.input === null ? '未知' : row.input.toLocaleString()}</strong></div><div><span>输出 Token</span><strong>{row.output === null ? '未知' : row.output.toLocaleString()}</strong></div></div><p className="mt-2 text-xs text-muted-foreground">{row.attempts.length > 1 ? '已知用量小计；首次尝试用量未知。' : row.input === null ? '上游未提供可确认用量，未以 0 代替。' : '用量来自这条虚拟记录的合成响应。'}</p></section>
          <details className="log-diagnostics" open={row.status === 'failed'}><summary>结构诊断<span className="mono ml-2 text-xs font-normal text-muted-foreground">{row.code ?? row.stage}</span></summary><dl className="mt-4 space-y-2"><div><dt>stage</dt><dd>{row.stage}</dd></div>{Object.entries(row.evidence).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{String(value)}</dd></div>)}</dl></details>
          <p className="flex gap-2 text-xs leading-6 text-muted-foreground"><ShieldCheck size={15} className="mt-1 shrink-0" />仅展示安全元数据；不包含请求正文、对话或密钥。此处为合成场景，不是那次报错的实际取证结果。</p>
        </div>
      </>}
    </SheetContent>
  </Sheet>;
}
