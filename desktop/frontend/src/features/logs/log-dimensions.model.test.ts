import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import fixture from '../../../../contract/log-dimensions.json';
import { checkedStats, statsQuery } from './log-stats.model';
import { initialFilters, identityQuery, safeSummary } from './log.model';
import { requestFixtures } from './log.mock';
import { LogGroups } from './log-groups.view';

it('shares grouped observations and keeps unknown separate from a real zero', () => {
  const data = checkedStats(structuredClone(fixture.stats));
  expect(data.groups?.map(g => [g.value, g.values.requests, g.values.cache_hit_rate])).toEqual([['anchor', 2, 80], ['', 1, 0]]);
  const html = renderToStaticMarkup(createElement(LogGroups, { data, dimension: 'project', onSelect: () => {} }));
  expect(html).toContain('项目分类汇总');
  expect(html).toContain('筛选项目：anchor');
  expect(html).toContain('筛选项目：未识别');
  expect(html).toContain('2 组');
});
it('keeps missing filter different from an explicit unknown across list/stat/export query builders', () => {
  expect(identityQuery(initialFilters)).toEqual({});
  const filters = { ...initialFilters, agent: 'codex', binding: 'desktop', project: '', session: 'thread-1' };
  expect(identityQuery(filters)).toEqual({ agent: 'codex', binding: 'desktop', project: '', session: 'thread-1' });
  expect(statsQuery(filters, 60000, 1800000000000, 'session')).toMatchObject({ group_by: 'session', filter: identityQuery(filters) });
  expect(safeSummary({ ...requestFixtures[0], agent: 'codex', project: '', session: 'thread-1' })).toContain('项目: 未识别');
});
it('rejects malformed dimensions, duplicates, group counts and observations', () => {
  const badDimension = { ...structuredClone(fixture.stats), group_by: 'document' };
  const duplicate = structuredClone(fixture.stats); duplicate.groups.push(duplicate.groups[0]);
  const badCount = { ...structuredClone(fixture.stats), group_count: 0 };
  const badMetric = structuredClone(fixture.stats); badMetric.groups[0].values.cache_hit_rate = 101;
  for (const data of [badDimension, duplicate, badCount, badMetric]) expect(() => checkedStats(data)).toThrow();
});
it('does not pretend an older core implements grouping', () => {
  const data = structuredClone(fixture.stats); Reflect.deleteProperty(data, 'group_by');
  const html = renderToStaticMarkup(createElement(LogGroups, { data, dimension: 'project', onSelect: () => {} }));
  expect(html).toContain('当前核心未提供分类统计');
});

it('keeps calling Key separate from supplier Key through filters, groups and summaries', () => {
  const filters = { ...initialFilters, access_key: 'caller-id', key: 'supplier-key' };
  expect(identityQuery(filters)).toEqual({ access_key: 'caller-id' });
  expect(statsQuery(filters, 60000, 1800000000000, 'access_key')).toMatchObject({ group_by: 'access_key', filter: { access_key: 'caller-id', key: 'supplier-key' } });
  const data = { ...structuredClone(fixture.stats), group_by: 'access_key' };
  expect(checkedStats(data).group_by).toBe('access_key');
  expect(safeSummary({ ...requestFixtures[0], access_key: 'caller-id', accessLabel: 'phone' })).toContain('调用 Key: caller-id');
  expect(safeSummary({ ...requestFixtures[0], accessLabel: 'phone' })).toContain('调用 Key 名称: phone');
});
