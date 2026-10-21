import { useId, useRef, useState, type PointerEvent } from 'react';
import { ChevronDown, ChevronUp, ListFilter } from 'lucide-react';
import { Button } from '../../components/ui/button';
import type { Stats } from '../../desktop/contract.generated';
import { dimensionLabels, type LogDimension, type LogFilters, type LogTimeRange } from './log.model';
import { LogGroups } from './log-groups.view';
import { bucketRange, chartPaths, formatStat, intervals, metrics, validInterval, type MetricID } from './log-stats.model';
import { useLogStatsController } from './use-log-stats-controller';
const width = 1000, height = 180, insetX = 4;
function TrendChart({ data, metric, range, onRangeSelect }: { data: Stats; metric: MetricID; range: LogTimeRange | null; onRangeSelect: (range: LogTimeRange | null) => void }) {
  const [active, setActive] = useState<number | null>(null);
  const [lastInspected, setLastInspected] = useState<number | null>(null);
  const anchor = useRef<number | null>(null), keyboardAnchor = useRef<number | null>(null);
  const [draft, setDraft] = useState<LogTimeRange | null>(null);
  const tooltipId = useId();
  const spec = metrics.find(m => m.id === metric)!;
  const stacked = metric === 'errors';
  const unit = stacked ? '次' : spec.unit;
  const series = stacked ? [
    { label: '成功次数', tone: 'success', values: data.buckets.map(b => b.values.completed - b.values.errors) },
    { label: '错误次数', tone: 'errors', values: data.buckets.map(b => b.values.errors) },
  ] : metric === 'tokens' ? [
    { label: '合计', tone: 'total', values: data.buckets.map(b => b.values.total_tokens) },
    { label: '输入', tone: 'input', values: data.buckets.map(b => b.values.input_tokens) },
    { label: '输出', tone: 'output', values: data.buckets.map(b => b.values.output_tokens) },
  ] : [{ label: spec.label, tone: metric, values: data.buckets.map(b => b.values[spec.field]) }];
  const known = series.flatMap(s => s.values.filter((v): v is number => v !== null));
  const max = stacked ? Math.max(2, Math.ceil(Math.max(...data.buckets.map(b => b.values.completed)) / 2) * 2)
    : metric === 'cache' ? 100 : Math.max(1, ...known) * 1.1;
  const index = Math.min(active ?? lastInspected ?? data.buckets.length - 1, data.buckets.length - 1);
  const point = data.buckets[index];
  const span = data.before_ms - data.since_ms;
  const positions = data.buckets.map(b => (b.at_ms - data.since_ms + Math.min(data.interval_ms, data.before_ms - b.at_ms) / 2) / span * width);
  const observation = (point.values.requests === 0 ? '此时段无请求' : series.every(s => s.values[index] === null) ? '此时段无可计算的指标' : `${point.values.requests} 次请求`)
    + (series.some(s => s.values[index] === null) ? '，缺失值按 0 显示' : '');
  const time = (at: number, detailed = false) => new Date(at).toLocaleString('zh-CN', { ...(data.before_ms - data.since_ms > 86400000 || detailed ? { month: '2-digit', day: '2-digit' } : {}), hour: '2-digit', minute: '2-digit', ...(data.interval_ms < 60000 ? { second: '2-digit' } : {}), hour12: false });
  const end = Math.min(point.at_ms + data.interval_ms, data.before_ms);
  const timeRange = `${time(point.at_ms, true)} – ${time(end, true)}`;
  const inspect = (next: number) => { setActive(next); setLastInspected(next); };
  const pointerIndex = (event: PointerEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return index;
    // Match the SVG viewBox padding so the cursor selects the bucket beneath it.
    const x = ((event.clientX - rect.left) / rect.width * (width + insetX * 2) - insetX) / width;
    return Math.max(0, Math.min(data.buckets.length - 1, Math.floor(x * span / data.interval_ms)));
  };
  const inspectPointer = (event: PointerEvent<HTMLDivElement>) => {
    const next = pointerIndex(event); inspect(next);
    if (anchor.current !== null) setDraft(bucketRange(data, anchor.current, next));
  };
  const selection = draft ?? range;
  const left = selection ? Math.max(data.since_ms, selection.since) : 0;
  const right = selection ? Math.min(data.before_ms, selection.before) : 0;
  return <>
    <div className="log-chart-legend"><span className="mono">{unit}</span>
      {series.map(s => <span key={s.label} data-tone={s.tone}><i />{s.label}</span>)}
    </div>
    <div className="log-chart-body">
      <div className="log-chart-y mono">{[max, max / 2, 0].map((v, i) => <span key={i}>{formatStat(v, true)}</span>)}</div>
      <div className="log-chart-plot" data-inspecting={active !== null} role="slider" tabIndex={0} aria-label={`${spec.label}趋势，左右方向键查看，回车选中，Shift 加方向键扩选，拖动选择区间`} aria-valuemin={0} aria-valuemax={data.buckets.length - 1} aria-valuenow={index}
        aria-valuetext={`${timeRange}，${observation}，${series.map(s => `${s.label} ${formatStat(s.values[index] ?? 0)} ${unit}`).join('，')}`}
        aria-describedby={tooltipId}
        onPointerEnter={inspectPointer} onPointerMove={inspectPointer}
        onPointerDown={e => { if (e.button !== 0 || !e.isPrimary) return; const next = pointerIndex(e); anchor.current = next; keyboardAnchor.current = null; inspect(next); setDraft(bucketRange(data, next)); e.currentTarget.setPointerCapture(e.pointerId); }}
        onPointerUp={e => { if (anchor.current === null) return; onRangeSelect(bucketRange(data, anchor.current, pointerIndex(e))); anchor.current = null; setDraft(null); if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId); }}
        onPointerCancel={() => { anchor.current = null; setDraft(null); }}
        onLostPointerCapture={() => { anchor.current = null; setDraft(null); }}
        onPointerLeave={() => setActive(null)} onFocus={() => inspect(index)} onBlur={() => setActive(null)}
        onKeyDown={e => {
          if (e.key === 'Escape') { e.preventDefault(); setActive(null); anchor.current = null; keyboardAnchor.current = null; setDraft(null); onRangeSelect(null); }
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); keyboardAnchor.current = index; onRangeSelect(bucketRange(data, index)); }
          if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
            e.preventDefault();
            const next = e.key === 'Home' ? 0 : e.key === 'End' ? data.buckets.length - 1 : Math.max(0, Math.min(data.buckets.length - 1, index + (e.key === 'ArrowLeft' ? -1 : 1)));
            if (e.shiftKey) { keyboardAnchor.current ??= index; onRangeSelect(bucketRange(data, keyboardAnchor.current, next)); } else keyboardAnchor.current = null;
            inspect(next);
          }
        }}>
        <svg viewBox={`${-insetX} -6 ${width + insetX * 2} ${height + 12}`} preserveAspectRatio="none" aria-hidden="true">
          {[0, height / 2, height].map(y => <line key={y} x1="0" x2={width} y1={y} y2={y} className="log-chart-grid" />)}
          {selection && right > left && <rect className="log-chart-range" x={(left - data.since_ms) / span * width} y="0" width={(right - left) / span * width} height={height} />}
          {active !== null && <rect className="log-chart-selection" x={(point.at_ms - data.since_ms) / span * width} y="0" width={Math.min(data.interval_ms, data.before_ms - point.at_ms) / span * width} height={height} />}
          {stacked && <defs>{data.buckets.map((bucket, i) => {
            const totalHeight = bucket.values.completed / max * height;
            const slotWidth = Math.min(data.interval_ms, data.before_ms - bucket.at_ms) / span * width;
            const barWidth = Math.min(40, slotWidth * .6);
            return <clipPath key={bucket.at_ms} id={`${tooltipId}-bar-${i}`}><rect x={positions[i] - barWidth / 2} y={height - totalHeight} width={barWidth} height={totalHeight} rx={Math.min(3, barWidth / 4, totalHeight / 2)} /></clipPath>;
          })}</defs>}
          {stacked ? data.buckets.map((bucket, i) => {
            const success = series[0].values[i]!;
            const errors = series[1].values[i]!;
            const slotWidth = Math.min(data.interval_ms, data.before_ms - bucket.at_ms) / span * width;
            const barWidth = Math.min(40, slotWidth * .6);
            const x = positions[i] - barWidth / 2;
            return <g key={bucket.at_ms} data-bucket={i} data-selected={active !== null && index === i} clipPath={`url(#${tooltipId}-bar-${i})`}>
              {success > 0 && <rect className="log-chart-bar" data-tone="success" x={x} y={height - success / max * height} width={barWidth} height={success / max * height} />}
              {errors > 0 && <rect className="log-chart-bar" data-tone="errors" x={x} y={height - (success + errors) / max * height} width={barWidth} height={errors / max * height} />}
            </g>;
          }) : series.map(s => <g key={s.label} data-tone={s.tone}>
            {chartPaths(s.values, max, width, height, positions).map((d, i) => <path key={i} d={d} className="log-chart-line" />)}
            {(active !== null || s.values.length === 1) && <circle cx={positions[active !== null ? index : 0]} cy={height - (s.values[active !== null ? index : 0] ?? 0) / max * height} r="3" className="log-chart-dot" />}
          </g>)}
          {!stacked && active !== null && <line x1={positions[index]} x2={positions[index]} y1="0" y2={height} className="log-chart-cursor" />}
        </svg>
      </div>
    </div>
    <div className="log-chart-x mono"><span>{time(data.since_ms)}</span><span>{time(data.since_ms + (data.before_ms - data.since_ms) / 2)}</span><span>{time(data.before_ms)}</span></div>
    <div id={tooltipId} role={active !== null ? 'tooltip' : undefined} className="log-chart-tooltip" aria-label="趋势读数">
      <div className="log-chart-reading-time">
        <div className="log-chart-tooltip-time mono"><time dateTime={new Date(point.at_ms).toISOString()}>{time(point.at_ms, true)}</time><span>–</span><time dateTime={new Date(end).toISOString()}>{time(end, true)}</time></div>
        <div className="log-chart-observation">{observation}</div>
      </div>
      <div className="log-chart-reading-values">{series.map(s => <div key={s.label} className="log-chart-tooltip-value" data-tone={s.tone}><span><i />{s.label}</span><strong className="mono">{formatStat(s.values[index] ?? 0)} {unit}</strong></div>)}</div>
      <Button variant="outline" size="sm" onClick={() => onRangeSelect(bucketRange(data, index))}><ListFilter size={14} />查看此时段</Button>
    </div>
  </>;
}
export function LogStatsView({ filters, paused, revision, onGroupSelect, range, onRangeSelect }: { filters: LogFilters; paused: boolean; revision: number; onGroupSelect: (dimension: LogDimension, value: string) => void; range: LogTimeRange | null; onRangeSelect: (range: LogTimeRange | null) => void }) {
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
      {!c.data || c.data.summary.requests === 0 ? <div className="log-stats-placeholder" role="status">{error || (!c.data && c.loading ? '趋势加载中' : '所选范围内暂无请求')}</div> : <TrendChart data={c.data} metric={c.metric} range={range} onRangeSelect={onRangeSelect} />}
      {c.groupBy && c.data && <LogGroups data={c.data} dimension={c.groupBy} onSelect={value => onGroupSelect(c.groupBy as LogDimension, value)} />}
    </div>
  </section>;
}
