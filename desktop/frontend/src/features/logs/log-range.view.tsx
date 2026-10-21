import { AlertCircle, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { errorExplanation, formatStat } from './log-stats.model';
import type { LogController } from './use-log-controller';
import { useLogRangeController } from './use-log-range-controller';

export function LogRangeView({ c }: { c: LogController }) {
  const state = useLogRangeController(c.filters, c.range, c.revision, c.paused);
  if (!c.range) return null;
  const errors = state.data?.error_summary;
  const time = (ms: number) => new Date(ms).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  return <section className="log-range log-surface" aria-label="选中区间" aria-busy={state.loading}>
    <header className="log-range-header">
      <div><h2>选中区间</h2><p className="mono"><time dateTime={new Date(c.range.since).toISOString()}>{time(c.range.since)}</time><span> — </span><time dateTime={new Date(c.range.before).toISOString()}>{time(c.range.before)}</time></p></div>
      <div className="log-range-actions"><Button variant={c.filters.status === 'errors' ? 'secondary' : 'outline'} size="sm" aria-pressed={c.filters.status === 'errors'} onClick={() => c.filter('status', c.filters.status === 'errors' ? 'all' : 'errors')}><AlertCircle size={14} />仅看错误</Button>
        <Button variant="ghost" size="icon" aria-label="清除区间选择" onClick={() => c.selectRange(null)}><X size={16} /></Button></div>
    </header>
    <div className="log-range-summary">
      <h3>区间错误{errors && <span> · {formatStat(errors.total)} 次 / {formatStat(errors.group_count)} 类</span>}</h3>
      <p className="log-range-note">按当前筛选汇总全部保留日志；时间以请求开始为准，包含起点、不含终点。仅提供安全错误码与阶段，不保存上游原始报错正文。</p>
      {state.loading ? <p role="status">错误汇总加载中…</p> : state.error ? <p role="status" className="text-destructive">{state.error}</p>
        : errors?.total === 0 ? <p role="status">此区间无失败或中断请求</p>
          : <><div className="log-error-groups">{errors?.groups.map(group => <div className="log-error-group" key={JSON.stringify([group.code, group.stage, group.http])}>
            <div className="log-error-description"><strong className="mono">{group.code || '未记录错误码'}</strong><span>{errorExplanation(group.code)}</span><small>{group.stage || '未记录阶段'} · HTTP {group.http ?? '—'} · 最近 {time(group.last_ms)}</small></div>
            <strong className="mono log-error-count">{formatStat(group.count)} 次</strong><Button variant="outline" size="sm" aria-label={`查看错误示例 ${group.code || '未记录错误码'} ${group.stage} HTTP ${group.http ?? '未知'}`} onClick={() => void c.findRequest(group.request_id)}>查看示例</Button>
          </div>)}</div>{errors && errors.group_count > errors.groups.length && <p role="status" className="log-range-note">展示次数最多的 {errors.groups.length} 类，共 {errors.group_count} 类；错误总数包含全部类型。</p>}</>}
    </div>
  </section>;
}
