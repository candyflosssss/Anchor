import { accountFailureText, amountText, platformLabels, allowanceScopeText, billingScopeHint } from './provider-account.model';
import type { ProviderBalance } from './use-provider-balances';

export function ProviderBalanceCell({ value }: { value?: ProviderBalance }) {
  if (!value) return <span className="cell-hint" role="status">查询中…</span>;
  const v = value.observation;
  const failure = value.error || (v?.failure ? accountFailureText[v.failure] ?? '查询失败，请稍后刷新。' : '');
  const platform = v && platformLabels[v.platform];
  if (failure) return <div title={failure}><span className="cell-hint">{v?.failure === 'auth_required' ? '需额外授权' : v?.failure === 'platform_unknown' ? '平台未识别' : '暂不可用'}</span>{platform && <span className="block text-xs text-muted-foreground">{platform}</span>}</div>;
  const a = v?.allowance;
  if (!v || !a) return <span className="cell-hint">未知</span>;
  const remaining = a.remaining ?? a.windows[0]?.remaining ?? null;
  return <div title={`${platform ?? '平台'} · ${allowanceScopeText(a.scope)}\nKey：${v.key_id}\n查询时间：${new Date(v.observed_at).toLocaleString()}${a.windows[0] ? `\n周期：${a.windows[0].period}` : ''}${a.scope === 'billing' ? `\n${billingScopeHint}` : ''}`}>
    <span className="block text-sm font-medium tabular-nums">{a.unlimited === true ? '不限额' : remaining ? amountText(remaining) : '未知'}</span>
    <span className="block text-xs text-muted-foreground">{platform} · {allowanceScopeText(a.scope)}{!a.remaining && a.windows[0] ? ` · ${a.windows[0].period}` : ''}</span>
  </div>;
}
