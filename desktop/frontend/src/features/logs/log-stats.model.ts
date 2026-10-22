import type { Stats, StatsQuery, StatsValues } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { identityQuery, type LogDimension, type LogFilters, type LogTimeRange } from './log.model';
export type MetricID = 'requests' | 'tokens' | 'tps' | 'errors' | 'cache' | 'latency' | 'first';
export const intervals = [
  { ms: 10000, label: '10 秒' }, { ms: 30000, label: '30 秒' }, { ms: 60000, label: '1 分钟' },
  { ms: 300000, label: '5 分钟' }, { ms: 900000, label: '15 分钟' }, { ms: 3600000, label: '1 小时' },
  { ms: 21600000, label: '6 小时' }, { ms: 86400000, label: '1 天' },
];
export const metrics: { id: MetricID; label: string; unit: string; field: keyof StatsValues }[] = [
  { id: 'requests', label: '请求量', unit: '次', field: 'requests' },
  { id: 'tokens', label: '已知用量', unit: 'Token', field: 'total_tokens' },
  { id: 'tps', label: '总 TPS', unit: 't/s', field: 'tps' },
  { id: 'errors', label: '错误率', unit: '%', field: 'error_rate' },
  { id: 'cache', label: '缓存命中率', unit: '%', field: 'cache_hit_rate' },
  { id: 'latency', label: '平均耗时', unit: 'ms', field: 'latency_ms' },
  { id: 'first', label: '首字延迟', unit: 'ms', field: 'first_output_ms' },
];
export function statsQuery(filters: LogFilters, interval: number, now = Date.now(), groupBy: LogDimension | '' = ''): StatsQuery {
  return { ...(groupBy ? { group_by: groupBy } : {}), interval_ms: interval, filter: { ...identityQuery(filters),
    kind: 'requests', limit: 50, cursor: 0, until: 0, since_ms: now - Number(filters.window) * 60000, before_ms: now,
    ...(filters.status === 'errors' ? { errors_only: true } : {}), status: ['all', 'errors'].includes(filters.status) ? '' : filters.status, source: filters.source === 'all' ? '' : filters.source,
    provider: filters.provider === 'all' ? '' : filters.provider, model: filters.model, key: filters.key,
    http: filters.http ? Number(filters.http) : null, code: filters.code, stage: filters.stage, level: '', search: filters.query.trim(),
  } };
}
export function validInterval(window: string, interval: number) { return interval === 0 || Math.ceil(Number(window) * 60000 / interval) <= 600; }
export function chartInterval(window: string, requested: number, metric: MetricID, compact: boolean) {
  if (requested && validInterval(window, requested)) return requested;
  if (metric !== 'errors') return 0;
  const limit = compact ? 24 : 48;
  return (intervals.find(i => Math.ceil(Number(window) * 60000 / i.ms) <= limit) ?? intervals.at(-1)!).ms;
}
export function checkedStats(value: Stats): Stats {
  if (!value || !Number.isSafeInteger(value.since_ms) || !Number.isSafeInteger(value.before_ms) || value.before_ms <= value.since_ms
    || !intervals.some(i => i.ms === value.interval_ms) || !Array.isArray(value.buckets) || value.buckets.length < 1 || value.buckets.length > 600
    || value.buckets.length !== Math.ceil((value.before_ms - value.since_ms) / value.interval_ms) || !value.health) throw new DesktopProblem('read_error');
  const check = (v: StatsValues) => {
    if (!v) throw new DesktopProblem('read_error');
    for (const key of ['requests', 'completed', 'errors', 'usage_samples', 'tps_samples', 'cache_samples', 'latency_samples', 'first_output_samples'] as const) {
      if (!Number.isSafeInteger(v[key]) || v[key] < 0) throw new DesktopProblem('read_error');
    }
    for (const key of ['input_tokens', 'output_tokens', 'total_tokens', 'tps', 'error_rate', 'cache_hit_rate', 'latency_ms', 'first_output_ms'] as const) {
      if (v[key] !== null && (typeof v[key] !== 'number' || !Number.isFinite(v[key]) || v[key] < 0)) throw new DesktopProblem('read_error');
    }
    if ((v.error_rate ?? 0) > 100 || (v.cache_hit_rate ?? 0) > 100) throw new DesktopProblem('read_error');
  };
  if (value.group_by != null && !['agent', 'binding', 'project', 'session', 'access_key'].includes(value.group_by)) throw new DesktopProblem('read_error');
  if (value.group_by) {
    const groups = value.groups ?? [];
    if (!Array.isArray(groups) || groups.length > 100 || !Number.isSafeInteger(value.group_count ?? 0) || (value.group_count ?? 0) < groups.length) throw new DesktopProblem('read_error');
    const seen = new Set<string>();
    for (const group of groups) {
      if (!group || typeof group.value !== 'string' || seen.has(group.value)) throw new DesktopProblem('read_error');
      seen.add(group.value); check(group.values);
    }
  }
  check(value.summary);
  if (value.error_summary != null) {
    const e = value.error_summary;
    if (!Array.isArray(e.groups) || e.groups.length > 100 || !Number.isSafeInteger(e.total) || e.total !== value.summary.errors
      || !Number.isSafeInteger(e.group_count) || e.group_count < e.groups.length || e.group_count > e.total || e.groups.length !== Math.min(100, e.group_count)) throw new DesktopProblem('read_error');
    const seen = new Set<string>();
    let count = 0;
    for (const g of e.groups) {
      const key = JSON.stringify([g.code, g.stage, g.http]);
      if (typeof g.code !== 'string' || typeof g.stage !== 'string' || typeof g.request_id !== 'string' || !g.request_id || seen.has(key)
        || (g.http !== null && (!Number.isInteger(g.http) || g.http < 100 || g.http > 599))
        || !Number.isSafeInteger(g.count) || g.count <= 0 || !Number.isSafeInteger(g.first_ms) || !Number.isSafeInteger(g.last_ms)
        || g.first_ms < value.since_ms || g.last_ms < g.first_ms || g.last_ms >= value.before_ms) throw new DesktopProblem('read_error');
      seen.add(key); count += g.count;
    }
    if (count + e.group_count - e.groups.length > e.total || (e.group_count <= 100 && count !== e.total)) throw new DesktopProblem('read_error');
  }
  value.buckets.forEach((b, i) => { if (b.at_ms !== value.since_ms + i * value.interval_ms) throw new DesktopProblem('read_error'); check(b.values); });
  return value;
}
export function errorStatsQuery(filters: LogFilters, range: LogTimeRange): StatsQuery {
  const query = statsQuery(filters, 0, range.before);
  query.filter.since_ms = range.since;
  query.include_errors = true;
  return query;
}
export function bucketRange(data: Stats, first: number, last = first): LogTimeRange {
  const clamp = (index: number) => Math.max(0, Math.min(data.buckets.length - 1, index));
  return { since: data.buckets[clamp(Math.min(first, last))].at_ms,
    before: Math.min(data.before_ms, data.buckets[clamp(Math.max(first, last))].at_ms + data.interval_ms) };
}
export { errorExplanation } from './log-error.model';
export function formatStat(value: number | null, compact = false) {
  if (value === null) return '—';
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1, ...(compact ? { notation: 'compact' } : {}) }).format(value);
}
export function chartPaths(values: (number | null)[], max: number, width = 1000, height = 180, positions?: number[]): string[] {
  if (!values.length) return [];
  const points = values.map((v, i) => ({
    x: positions?.[i] ?? (values.length === 1 ? width / 2 : i * width / (values.length - 1)),
    y: height - (v ?? 0) / max * height,
  }));
  const coordinate = (x: number, y: number) => `${x.toFixed(2)},${y.toFixed(2)}`;
  let path = `M${coordinate(points[0].x, points[0].y)}`;
  if (points.length === 1) return [path];
  const spans = points.slice(1).map((p, i) => p.x - points[i].x);
  const slopes = spans.map((span, i) => (points[i + 1].y - points[i].y) / span);
  // PCHIP uses shape-preserving derivatives, including on unequal time intervals.
  // Flat intervals and extrema have zero slope, preventing overshoot below zero.
  const endpoint = (h0: number, h1: number, d0: number, d1: number) => {
    const slope = ((2 * h0 + h1) * d0 - h0 * d1) / (h0 + h1);
    if (Math.sign(slope) !== Math.sign(d0)) return 0;
    return Math.sign(d0) !== Math.sign(d1) && Math.abs(slope) > 3 * Math.abs(d0) ? 3 * d0 : slope;
  };
  const derivatives = points.map((_, i) => {
    if (points.length === 2) return slopes[0];
    if (i === 0) return endpoint(spans[0], spans[1], slopes[0], slopes[1]);
    if (i === points.length - 1) return endpoint(spans[i - 1], spans[i - 2], slopes[i - 1], slopes[i - 2]);
    if (slopes[i - 1] * slopes[i] <= 0) return 0;
    const w1 = 2 * spans[i] + spans[i - 1], w2 = spans[i] + 2 * spans[i - 1];
    return (w1 + w2) / (w1 / slopes[i - 1] + w2 / slopes[i]);
  });
  points.slice(1).forEach((p, i) => {
    const previous = points[i], third = spans[i] / 3;
    path += ` C${coordinate(previous.x + third, previous.y + derivatives[i] * third)} ${coordinate(p.x - third, p.y - derivatives[i + 1] * third)} ${coordinate(p.x, p.y)}`;
  });
  return [path];
}
