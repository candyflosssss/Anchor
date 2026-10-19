import type { ReactNode } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { accountFailureText, amountText, platformLabels, allowanceScopeText, billingScopeHint } from './provider-account.model';
import type { ProviderBalance } from './provider-balance.model';

export function ProviderBalanceCell({ value, providerName, onRefresh, disabled }: {
  value?: ProviderBalance; providerName?: string; onRefresh?: () => void; disabled?: boolean;
}) {
  const v = value?.observation;
  const failureCode = value?.failure || v?.failure;
  const failure = value?.error || (failureCode ? accountFailureText[failureCode] ?? '查询失败，请稍后刷新。' : '');
  const platform = v && platformLabels[v.platform];
  const a = v?.allowance;
  let content: ReactNode;
  if (v && a) {
    const remaining = a.remaining ?? a.windows[0]?.remaining ?? null;
    content = <div title={`${platform ?? '平台'} · ${allowanceScopeText(a.scope)}\nKey：${v.key_id}\n查询时间：${new Date(v.observed_at).toLocaleString()}${a.windows[0] ? `\n周期：${a.windows[0].period}` : ''}${a.scope === 'billing' ? `\n${billingScopeHint}` : ''}`}>
      <span className="block text-sm font-medium tabular-nums">{a.unlimited === true ? '不限额' : remaining ? amountText(remaining) : '未知'}</span>
      <span className="block text-xs text-muted-foreground">{platform} · {allowanceScopeText(a.scope)}{!a.remaining && a.windows[0] ? ` · ${a.windows[0].period}` : ''}</span>
    </div>;
  } else if (failure) {
    content = <div title={failure}><span className="cell-hint">{failureCode === 'auth_required' ? '需额外授权' : failureCode === 'platform_unknown' ? '平台未识别' : '暂不可用'}</span>{platform && <span className="block text-xs text-muted-foreground">{platform}</span>}</div>;
  } else {
    content = <span className="cell-hint" role="status">{!value || value.refreshing ? '查询中…' : '未知'}</span>;
  }
  return <div className="flex items-center gap-2" aria-busy={!!value?.refreshing}>
    <div className="min-w-0">{content}{failure && <span className="block text-xs text-destructive" role="status" title={failure}>余额未刷新</span>}</div>
    {onRefresh && <Button type="button" variant="ghost" size="icon" className="h-8 w-8 shrink-0 text-muted-foreground" disabled={disabled || !!value?.refreshing}
      aria-label={`刷新 ${providerName ?? '供应商'} 的余额`} title={value?.refreshing ? '正在刷新余额' : failure ? `刷新余额：${failure}` : '刷新余额'} onClick={onRefresh}>
      <RefreshCw className={value?.refreshing ? 'h-3.5 w-3.5 animate-spin motion-reduce:animate-none' : 'h-3.5 w-3.5'} aria-hidden="true" />
    </Button>}
  </div>;
}
