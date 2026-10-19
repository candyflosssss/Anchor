import { useId, useState, type PointerEvent } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { Button } from '../../components/ui/button';
import type { Stats } from '../../desktop/contract.generated';
import { dimensionLabels, type LogDimension, type LogFilters } from './log.model';
import { LogGroups } from './log-groups.view';
import { chartPaths, formatStat, intervals, metrics, validInterval, type MetricID } from './log-stats.model';
import { useLogStatsController } from './use-log-stats-controller';
const width = 1000, height = 180, insetX = 4;
function TrendChart({ data, metric }: { data: Stats; metric: MetricID }) {
  const [active, setActive] = useState<number | null>(null);
  const tooltipId = useId();
  const spec = metrics.find(m => m.id === metric)!;
  const series = metric === 'tokens' ? [
    { label: '合计', tone: 'total', values: data.buckets.map(b => b.values.total_tokens) },
    { label: '输入', tone: 'input', values: data.buckets.map(b => b.values.input_tokens) },
    { label: '输出', tone: 'output', values: data.buckets.map(b => b.values.output_tokens) },
  ] : [{ label: spec.label, tone: metric, values: data.buckets.map(b => b.values[spec.field]) }];
  const known = series.flatMap(s => s.values.filter((v): v is number => v !== null));
  const max = metric === 'errors' || metric === 'cache' ? 100 : Math.max(1, ...known) * 1.1;
  const index = active === null ? data.buckets.length - 1 : Math.min(active, data.buckets.length - 1);
  const point = data.buckets[index];
  const span = data.before_ms - data.since_ms;
  const positions = data.buckets.map(b => (b.at_ms - data.since_ms + Math.min(data.interval_ms, data.before_ms - b.at_ms) / 2) / span * width);
  const observation = (point.values.requests === 0 ? '此时段无请求' : series.every(s => s.values[index] === null) ? '此时段无可计算的指标' : `${point.values.requests} 次请求`)
    + (series.some(s => s.values[index] === null) ? '，缺失值按 0 显示' : '');
  const time = (at: number, detailed = false) => new Date(at).toLocaleString('zh-CN', { ...(data.before_ms - data.since_ms > 86400000 || detailed ? { month: '2-digit', day: '2-digit' } : {}), hour: '2-digit', minute: '2-digit', ...(data.interval_ms < 60000 ? { second: '2-digit' } : {}), hour12: false });
  const end = Math.min(point.at_ms + data.interval_ms, data.before_ms);
  const timeRange = `${time(point.at_ms, true)} – ${time(end, true)}`;
  const anchor = (positions[index] + insetX) / (width + insetX * 2) * 100;
  const inspectPointer = (event: PointerEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    // Match the SVG viewBox padding so the cursor selects the bucket beneath it.
    const x = ((event.clientX - rect.left) / rect.width * (width + insetX * 2) - insetX) / width;
    setActive(Math.max(0, Math.min(data.buckets.length - 1, Math.floor(x * span / data.interval_ms))));
  };
  return <>
    <div className="log-chart-legend"><span className="mono">{spec.unit}</span>
      {series.map(s => <span key={s.label} data-tone={s.tone}><i />{s.label}</span>)}
    </div>
    <div className="log-chart-body">
      <div className="log-chart-y mono">{[max, max / 2, 0].map((v, i) => <span key={i}>{formatStat(v, true)}</span>)}</div>
      <div className="log-chart-plot" role="slider" tabIndex={0} aria-label={`${spec.label}趋势，左右方向键查看时间点`} aria-valuemin={0} aria-valuemax={data.buckets.length - 1} aria-valuenow={index}
        aria-valuetext={`${timeRange}，${observation}，${series.map(s => `${s.label} ${formatStat(s.values[index] ?? 0)} ${spec.unit}`).join('，')}`}
        aria-describedby={active !== null ? tooltipId : undefined}
        onPointerEnter={inspectPointer} onPointerMove={inspectPointer}
        onPointerLeave={() => setActive(null)} onFocus={() => setActive(current => current ?? data.buckets.length - 1)} onBlur={() => setActive(null)}
        onKeyDown={e => {
          if (e.key === 'Escape') { e.preventDefault(); setActive(null); }
          if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) { e.preventDefault(); setActive(e.key === 'Home' ? 0 : e.key === 'End' ? data.buckets.length - 1 : Math.max(0, Math.min(data.buckets.length - 1, index + (e.key === 'ArrowLeft' ? -1 : 1)))); }
        }}>
        <svg viewBox={`${-insetX} -6 ${width + insetX * 2} ${height + 12}`} preserveAspectRatio="none" aria-hidden="true">
          {[0, height / 2, height].map(y => <line key={y} x1="0" x2={width} y1={y} y2={y} className="log-chart-grid" />)}
          {series.map(s => <g key={s.label} data-tone={s.tone}>
            {chartPaths(s.values, max, width, height, positions).map((d, i) => <path key={i} d={d} className="log-chart-line" />)}
            {s.values.length === 1 && <circle cx={positions[0]} cy={height - (s.values[0] ?? 0) / max * height} r="3" className="log-chart-dot" />}
          </g>)}
          {active !== null && <line x1={positions[index]} x2={positions[index]} y1="0" y2={height} className="log-chart-cursor" />}
        </svg>
        {active !== null && <div id={tooltipId} role="tooltip" className="log-chart-tooltip" style={{ left: `${anchor}%`, transform: `translateX(-${anchor}%)` }}>
          <div className="log-chart-tooltip-time mono"><time dateTime={new Date(point.at_ms).toISOString()}>{time(point.at_ms, true)}</time><span>–</span><time dateTime={new Date(end).toISOString()}>{time(end, true)}</time></div>
          <div className="log-chart-observation">{observation}</div>
          {series.map(s => <div key={s.label} className="log-chart-tooltip-value" data-tone={s.tone}><span><i />{s.label}</span><strong className="mono">{formatStat(s.values[index] ?? 0)} {spec.unit}</strong></div>)}
        </div>}
      </div>
    </div>
    <div className="log-chart-x mono"><span>{time(data.since_ms)}</span><span>{time(data.since_ms + (data.before_ms - data.since_ms) / 2)}</span><span>{time(data.before_ms)}</span></div>
  </>;
}
export function LogStatsView({ filters, paused, revision, onGroupSelect }: { filters: LogFilters; paused: boolean; revision: number; onGroupSelect: (dimension: LogDimension, value: string) => void }) {
  const c = useLogStatsController(filters, paused, revision);
  const [collapsed, setCollapsed] = useState(false);
  const contentId = useId();
  const error = c.error ? `${c.error}${c.data ? '；保留上次读取结果。' : ''}` : '';
  return <section className="log-stats log-surface" aria-label="请求运行趋势" aria-busy={c.loading} data-collapsed={collapsed}>
    <header className="log-stats-header"><h2>运行趋势</h2>{!collapsed && <><label>分类统计<select aria-label="日志分类统计" className="select-field" value={c.groupBy} onChange={e => c.setGroupBy(e.target.value as LogDimension | '')}>
      <option value="">不分类</option>{(Object.keys(dimensionLabels) as LogDimension[]).map(key => <option key={key} value={key}>按{dimensionLabels[key]}</option>)}
    </select></label><label>统计粒度<select aria-label="趋势统计粒度" className="select-field" value={c.interval} onChange={e => c.setInterval(Number(e.target.value))}>
      <option value={0}>自动{c.data && c.interval === 0 ? ` · ${intervals.find(i => i.ms === c.data!.interval_ms)?.label ?? ''}` : ''}</option>
      {intervals.map(i => <option key={i.ms} value={i.ms} disabled={!validInterval(filters.window, i.ms)}>{i.label}</option>)}
    </select></label></>}
      <Button variant="ghost" size="sm" type="button" aria-label={collapsed ? '展开运行趋势' : '折叠运行趋势'} aria-expanded={!collapsed} aria-controls={contentId} onClick={() => setCollapsed(value => !value)}>
        {collapsed ? <ChevronDown size={14} /> : <ChevronUp size={14} />}{collapsed ? '展开' : '折叠'}
      </Button>
    </header>
    <div id={contentId} hidden={collapsed}>
      <div className="log-stats-metrics" role="group" aria-label="趋势指标">{metrics.map(m => <button key={m.id} aria-pressed={c.metric === m.id} data-tone={m.id} onClick={() => c.setMetric(m.id)}>
        <span>{m.label}</span><strong>{formatStat(c.data?.summary[m.field] ?? null, true)}<small>{m.unit}</small></strong>
      </button>)}</div>
      {error && c.data && c.data.summary.requests > 0 && <p className="px-4 text-sm text-destructive" role="status">{error}</p>}
      {!c.data || c.data.summary.requests === 0 ? <div className="log-stats-placeholder" role="status">{error || (!c.data && c.loading ? '趋势加载中' : '所选范围内暂无请求')}</div> : <TrendChart data={c.data} metric={c.metric} />}
      {c.groupBy && c.data && <LogGroups data={c.data} dimension={c.groupBy} onSelect={value => onGroupSelect(c.groupBy as LogDimension, value)} />}
    </div>
  </section>;
}
