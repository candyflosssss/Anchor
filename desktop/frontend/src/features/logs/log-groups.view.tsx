import type { Stats } from '../../desktop/contract.generated';
import { dimensionLabels, type LogDimension } from './log.model';
import { formatStat } from './log-stats.model';

export function LogGroups({ data, dimension, onSelect }: { data: Stats; dimension: LogDimension; onSelect: (value: string) => void }) {
  if (data.group_by !== dimension) return <p role="status" className="log-group-status">当前核心未提供分类统计，请更新核心版本</p>;
  if (!data.groups?.length) return null;
  return <section className="log-groups" aria-label={`${dimensionLabels[dimension]}分类汇总`}>
    <header><h3>{dimensionLabels[dimension]}分类汇总</h3><span>{data.group_count} 组{(data.group_count ?? 0) > data.groups.length ? ` · 显示请求最多的 ${data.groups.length} 组` : ''}</span></header>
    <div className="log-groups-scroll"><table><thead><tr>{[dimensionLabels[dimension], '请求量', '已知 Token', '总 TPS', '错误率 %', '缓存命中率 %'].map(label => <th key={label}>{label}</th>)}</tr></thead>
      <tbody>{data.groups.map(group => <tr key={group.value}>
        <th scope="row"><button type="button" onClick={() => onSelect(group.value)} aria-label={`筛选${dimensionLabels[dimension]}：${group.value || '未识别'}`}>{group.value || '未识别'}</button></th>
        <td>{formatStat(group.values.requests)}</td><td>{formatStat(group.values.total_tokens)}</td><td>{formatStat(group.values.tps)}</td><td>{formatStat(group.values.error_rate)}</td><td>{formatStat(group.values.cache_hit_rate)}</td>
      </tr>)}</tbody></table></div>
  </section>;
}
