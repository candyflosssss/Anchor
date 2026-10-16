import { useRef } from 'react';
import { Copy } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../../components/ui/sheet';
import { statusLabels, sourceLabels, serviceTierLabel, type RequestLog } from './log.model';
import { tierSnapshot } from './log-service-tier.model';
import { TierValue } from './log-service-tier.view';
import { logColumns } from './log-columns.model';
import { ColumnValue } from './log-request-table.view';
export function RequestDetail({ row, onClose, onCopy, notice }: { row: RequestLog | null; onClose: () => void; onCopy: (row: RequestLog) => void; notice: string }) {
  const opener = useRef<HTMLElement | null>(null);
  return <Sheet open={row !== null} onOpenChange={open => { if (!open) onClose(); }}><SheetContent aria-describedby={undefined} className="log-detail-sheet" closeLabel="关闭请求详情"
    onOpenAutoFocus={() => { opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; }}
    onCloseAutoFocus={event => { event.preventDefault(); opener.current?.focus(); }}>
    {row && <><SheetHeader><SheetTitle>请求详情</SheetTitle><div className="flex justify-between gap-2 text-sm"><span>{statusLabels[row.status]} · {sourceLabels[row.source]}</span><Button variant="outline" size="sm" onClick={() => onCopy(row)}><Copy size={13} />复制</Button></div></SheetHeader>
      <div className="log-detail-body">{notice && <p role="status">{notice}</p>}
        <section><h3 className="log-section-title">请求</h3><dl className="log-properties">{logColumns.map(col => <div key={col.id}><dt>{col.label}</dt><dd><ColumnValue id={col.id} row={row} /></dd></div>)}</dl></section>
        <section aria-label="服务档位"><h3 className="log-section-title">服务档位</h3><dl className="log-properties">
          <div><dt>请求档位</dt><dd><TierValue tier={tierSnapshot(row.evidence, 'requested')} /></dd></div>
          {row.evidence.mapping_service_tier && <div><dt>Anchor 固定档位</dt><dd>{serviceTierLabel(row.evidence.mapping_service_tier)}</dd></div>}
          <div><dt>上游返回档位（最后候选）</dt><dd><TierValue tier={tierSnapshot(row.evidence, 'upstream')} /></dd></div>
        </dl></section>
        <section aria-label="尝试时间线"><h3 className="log-section-title">尝试时间线 · {row.attemptCount ?? row.attempts.length}</h3>
          {row.attempts.length === 0 ? <p>暂无尝试</p> : <ol className="log-timeline">{row.attempts.map(a => <li key={a.number}><span className="log-attempt-dot">{a.number}</span><dl className="log-properties log-attempt-card">{Object.entries({ '供应商':a.provider, Key:a.key, '模型':a.model, '协议':a.protocol, HTTP:a.http ?? '未知', '耗时':`${a.elapsed} ms`, '输入 Token':a.input ?? '未知', '输出 Token':a.output ?? '未知', '结果':a.outcome, '错误码':a.code ?? '—', '阶段':a.note }).map(([k,v]) => <div key={k}><dt>{k}</dt><dd className="mono">{v}</dd></div>)}</dl></li>)}</ol>}
          {row.attemptsTruncated && <p role="status">尝试明细已截断</p>}
        </section>
        <section><h3 className="log-section-title">结构诊断</h3><dl className="log-properties">{Object.entries(row.evidence).map(([k,v]) => <div key={k}><dt>{k}</dt><dd>{String(v)}</dd></div>)}</dl></section>
      </div></>}
  </SheetContent></Sheet>;
}
