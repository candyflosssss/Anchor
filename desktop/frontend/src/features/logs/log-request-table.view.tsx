import { ChevronRight } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { outputTps, type LogColumnId } from './log-columns.model';
import { sourceLabels, statusLabels, type RequestLog } from './log.model';
import type { LogController } from './use-log-controller';
import type { LogColumnsController } from './use-log-columns-controller';

const number = (value: number | null | undefined) => value == null ? '未知' : value.toLocaleString();
const milliseconds = (value: number | null | undefined) => value == null ? '未知' : `${value.toLocaleString()} ms`;
function Metric({ label, value }: { label: string; value: string }) {
  return <div><dt>{label}</dt><dd className="mono">{value}</dd></div>;
}
export function ColumnValue({ id, row }: { id: LogColumnId; row: RequestLog }) {
  switch (id) {
    case 'time': return <div className="log-time"><span className="mono">{row.time}</span><small>{row.ageMinutes} 分钟前</small></div>;
    case 'status': return <div className="log-info-block">
      <div className="log-result-line"><span className={`log-result log-result-${row.status}`}><span className="mini-dot" />{row.status === 'failed' && ['invalid_response', 'unexpected_html'].includes(row.code ?? '') ? '校验失败' : statusLabels[row.status]}</span><span className="log-http mono">{row.http === null ? 'HTTP 未收到' : `HTTP ${row.http}`}</span></div>
      {row.code && <span className="mono log-block-primary text-destructive text-[10px]">{row.code}</span>}
      <dl className="log-meta-list"><Metric label="生成" value={row.generationStatus ?? '未知'} /><Metric label="交付" value={row.delivery} /></dl>
      {(row.attemptCount ?? row.attempts.length) > 1 && <small className="log-subtext">{row.attemptCount ?? row.attempts.length} 次尝试</small>}
    </div>;
    case 'model': return <div className="log-info-block"><span className="mono font-medium log-block-primary">{row.model}</span><dl className="log-meta-list">
      <Metric label="实际" value={row.upstreamModel ?? row.attempts.at(-1)?.model ?? '未知'} />
      <Metric label="思考" value={row.reasoningEffort === null ? '未指定' : row.reasoningEffort ?? '未知'} />
    </dl></div>;
    case 'requestId': return <span className="mono">{row.id}</span>;
    case 'source': return <>{sourceLabels[row.source]}</>;
    case 'provider': return <div className="log-info-block"><span className="font-medium log-block-primary">{row.provider}</span><dl className="log-meta-list"><Metric label="Key" value={row.key} /></dl></div>;
    case 'protocol': return <div className="log-info-block"><span className="mono log-block-primary">{row.protocol}</span><dl className="log-meta-list"><Metric label="返回" value={row.stream == null ? '未知' : row.stream ? '流式' : '非流式'} /></dl></div>;
    case 'revision': return <span className="mono">{row.revision > 0 ? row.revision : '未知'}</span>;
    case 'timing': {
      const rate = outputTps(row);
      return <dl className="log-metric-grid" aria-label="耗时与速度">
        <Metric label="总耗时" value={milliseconds(row.elapsed)} />
        <Metric label="首字" value={milliseconds(row.metrics?.timeToFirstOutputMs ?? row.metrics?.timeToFirstTextMs)} />
        <Metric label="输出" value={milliseconds(row.stream && row.metrics?.outputIntervalComplete ? row.metrics.outputGenerationMs ?? null : null)} />
        <Metric label="输出TPS" value={rate === null ? '未知' : `${rate.toFixed(1)} t/s`} />
      </dl>;
    }
    case 'tokens': return <div className="log-token-block"><dl className="log-metric-grid" aria-label="Token 用量">
      <Metric label="输入" value={number(row.input)} />
      <Metric label="输出" value={number(row.output)} />
      <Metric label="缓存命中" value={number(row.metrics?.cacheReadTokens)} />
      <Metric label="思考" value={number(row.metrics?.reasoningTokens)} />
    </dl><div className="log-token-total"><span>{(row.attemptCount ?? row.attempts.length) > 1 ? '已知小计' : '合计'}</span><span className="mono">{number(row.input == null || row.output == null ? null : row.input + row.output)}</span></div></div>;
    case 'attempts': return <span className="mono">{row.attemptCount ?? row.attempts.length}</span>;
    case 'diagnostics': return <div className="log-info-block"><span className={`mono log-block-primary ${row.code ? 'text-destructive' : 'text-muted-foreground'}`}>{row.code ?? '—'}</span><dl className="log-meta-list"><Metric label="阶段" value={row.stage} /></dl></div>;
  }
}
export function RequestTable({ c, layout }: { c: LogController; layout: LogColumnsController }) {
  return <Table className="log-table log-custom-table" style={{ minWidth: layout.columns.reduce((width, column) => width + column.width, 48) }}>
    <TableHeader><TableRow>
      {layout.columns.map(column => <TableHead key={column.id} style={{ width: column.width }}>{column.label}</TableHead>)}
      <TableHead className="log-detail-column"><span className="sr-only">详情</span></TableHead>
    </TableRow></TableHeader>
    <TableBody>{c.visible.map(row => <TableRow key={row.id} className="log-request-row" onClick={() => c.select(row)}>
      {layout.columns.map(column => <TableCell key={column.id}><ColumnValue id={column.id} row={row} /></TableCell>)}
      <TableCell className="log-detail-column"><Button variant="ghost" size="icon" aria-label={`查看请求 ${row.id}`} onClick={event => { event.stopPropagation(); c.select(row); }}><ChevronRight size={14} /></Button></TableCell>
    </TableRow>)}</TableBody>
  </Table>;
}
