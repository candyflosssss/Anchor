import type { KeyStatus } from '../../desktop/contract.generated';

/** Display only core-derived costs; views never calculate routing weights. */
export function KeyMultiplierView({ value }: { value?: KeyStatus }) {
  const rate = value?.rate;
  return <div className="key-multiplier">
    <span>分组倍率 <strong>{rate?.multiplier != null ? `${rate.multiplier}×` : '未知'}</strong></span>
    {' · '}<span>相对倍率 <strong>{value?.effective_multiplier != null ? `${value.effective_multiplier}×` : '未知'}</strong></span>
    {rate?.observed_at && <small className="card-hint" title={`最近查询：${rate.attempted_at}`}> · 缓存于 {new Date(rate.observed_at).toLocaleString('zh-CN')}</small>}
    {rate?.failure && <p className="card-hint">{rate.multiplier != null ? '刷新失败，沿用上次成功缓存。' : '尚无有效缓存；倍率未知，不按 1× 处理。'}</p>}
  </div>;
}
