import type { Stats, StatsQuery, StatsValues } from '../../desktop/contract.generated';
import { DesktopProblem } from '../../desktop/transport';
import { identityQuery, type LogDimension, type LogFilters } from './log.model';
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
    status: filters.status === 'all' ? '' : filters.status, source: filters.source === 'all' ? '' : filters.source,
    provider: filters.provider === 'all' ? '' : filters.provider, model: filters.model, key: filters.key,
    http: filters.http ? Number(filters.http) : null, code: filters.code, stage: filters.stage, level: '', search: filters.query.trim(),
  } };
}
export function validInterval(window: string, interval: number) { return interval === 0 || Math.ceil(Number(window) * 60000 / interval) <= 600; }
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
  if (value.group_by != null && !['agent', 'binding', 'project', 'session'].includes(value.group_by)) throw new DesktopProblem('read_error');
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
  value.buckets.forEach((b, i) => { if (b.at_ms !== value.since_ms + i * value.interval_ms) throw new DesktopProblem('read_error'); check(b.values); });
  return value;
}
export function formatStat(value: number | null, compact = false) {
  if (value === null) return '—';
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1, ...(compact ? { notation: 'compact' } : {}) }).format(value);
}
export function chartPaths(values: (number | null)[], max: number, width = 1000, height = 180, positions?: number[]): string[] {
  const paths: string[] = []; let path = '';
  values.forEach((v, i) => {
    if (v === null) { if (path) paths.push(path); path = ''; return; }
    const x = positions?.[i] ?? (values.length === 1 ? width / 2 : i * width / (values.length - 1)), y = height - v / max * height;
    path += `${path ? ' L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`;
  });
  if (path) paths.push(path);
  return paths;
}

// Bridge only idle buckets between observations. A request with an unknown
// measurement is a hard break; these paths never create interpolated values.
export function idleBridges(values: (number | null)[], requests: number[], max: number, width = 1000, height = 180, positions?: number[]): string[] {
  const bridges: string[] = [];
  let previous: number | null = null;
  values.forEach((value, index) => {
    if (value === null) {
      if (requests[index] !== 0) previous = null;
      return;
    }
    if (previous !== null && index > previous + 1) {
      const x = (i: number) => positions?.[i] ?? (values.length === 1 ? width / 2 : i * width / (values.length - 1));
      bridges.push(`M${x(previous).toFixed(2)},${(height - values[previous]! / max * height).toFixed(2)} L${x(index).toFixed(2)},${(height - value / max * height).toFixed(2)}`);
    }
    previous = index;
  });
  return bridges;
}
