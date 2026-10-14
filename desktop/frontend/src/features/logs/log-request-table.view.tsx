import { ChevronRight } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table';
import { outputTps, type LogColumnId } from './log-columns.model';
import { sourceLabels, statusLabels, type RequestLog } from './log.model';
import type { LogController } from './use-log-controller';
import type { LogColumnsController } from './use-log-columns-controller';

const number = (value: number | null | undefined) => value == null ? '未知' : value.toLocaleString();
const milliseconds = (value: number | null | undefined) => value == null ? '未知' : `${value.toLocaleString()} ms`;
export function ColumnValue({ id, row }: { id: LogColumnId; row: RequestLog }) {
  switch (id) {
    case 'time': return <div className="log-time"><span className="mono">{row.time}</span><small>{row.ageMinutes} 分钟前</small></div>;
    case 'status': return <><span className={`log-result log-result-${row.status}`}><span className="mini-dot" />{row.status === 'failed' && ['invalid_response', 'unexpected_html'].includes(row.code ?? '') ? '校验失败' : statusLabels[row.status]}</span>{(row.attemptCount ?? row.attempts.length) > 1 && <small className="log-subtext">{(row.attemptCount ?? row.attempts.length)} 次尝试</small>}</>;
    case 'model': return <span className="mono font-medium">{row.model}</span>;
    case 'requestId': return <span className="mono">{row.id}</span>;
    case 'source': return <>{sourceLabels[row.source]}</>;
    case 'upstreamModel': return <span className="mono">{row.upstreamModel ?? row.attempts.at(-1)?.model ?? '未知'}</span>;
    case 'provider': return <>{row.provider}</>;
    case 'key': return <span className="mono">{row.key}</span>;
    case 'protocol': return <>{row.protocol}</>;
    case 'reasoningEffort': return <span className="mono">{row.reasoningEffort === null ? '未指定' : row.reasoningEffort ?? '未知'}</span>;
    case 'stream': return <>{row.stream == null ? '未知' : row.stream ? '是' : '否'}</>;
    case 'revision': return <span className="mono">{row.revision > 0 ? row.revision : "未知"}</span>;
    case 'ttft': return <span className="mono">{milliseconds(row.metrics?.timeToFirstTextMs)}</span>;
    case 'elapsed': return <span className="mono">{milliseconds(row.elapsed)}</span>;
    case 'generationMs': return <span className="mono">{milliseconds(row.stream && row.metrics?.textIntervalComplete ? row.metrics.generationMs : null)}</span>;
    case 'tps': { const rate = outputTps(row); return <span className="mono">{rate === null ? '未知' : `${rate.toFixed(1)} t/s`}</span>; }
    case 'input':
    case 'output':
    case 'totalTokens': {
      const value = id === 'input' ? row.input : id === 'output' ? row.output : row.input === null || row.output === null ? null : row.input + row.output;
      return <><span className="mono">{number(value)}</span></>;
    }
    case 'cacheReadTokens': return <span className="mono">{number(row.metrics?.cacheReadTokens)}</span>;
    case 'reasoningTokens': return <span className="mono">{number(row.metrics?.reasoningTokens)}</span>;
    case 'attempts': return <span className="mono">{(row.attemptCount ?? row.attempts.length)}</span>;
    case 'http': return <span className="mono">{row.http === null ? '未收到' : `HTTP ${row.http}`}</span>;
    case 'errorCode': return <span className={`mono ${row.code ? 'text-destructive' : 'text-muted-foreground'}`}>{row.code ?? '—'}</span>;
    case 'stage': return <span className="mono">{row.stage}</span>;
    case 'generationStatus': return <span className="mono">{row.generationStatus ?? '未知'}</span>;
    case 'delivery': return <>{row.delivery}</>;
  }
}
export function RequestTable({ c, layout }: { c: LogController; layout: LogColumnsController }) {
  return <Table className="log-table log-custom-table" style={{ minWidth: layout.columns.reduce((width, column) => width + column.width, 48) }}>
    <TableHeader><TableRow>
      {layout.columns.map(column => <TableHead key={column.id} style={{ width: column.width }} className={'numeric' in column && column.numeric ? 'text-right' : undefined}>{column.label}</TableHead>)}
      <TableHead className="log-detail-column"><span className="sr-only">详情</span></TableHead>
    </TableRow></TableHeader>
    <TableBody>{c.visible.map(row => <TableRow key={row.id} className="log-request-row" onClick={() => c.select(row)}>
      {layout.columns.map(column => <TableCell key={column.id} className={'numeric' in column && column.numeric ? 'text-right' : undefined}><ColumnValue id={column.id} row={row} /></TableCell>)}
      <TableCell className="log-detail-column"><Button variant="ghost" size="icon" aria-label={`查看请求 ${row.id}`} onClick={event => { event.stopPropagation(); c.select(row); }}><ChevronRight size={14} /></Button></TableCell>
    </TableRow>)}</TableBody>
  </Table>;
}
