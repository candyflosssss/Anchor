import { rateBasisText, rateFailureText, type AccountRateObservation } from './provider-account.model';
import type { KeyRateCache, KeyStatus } from '../../desktop/contract.generated';
import { KeyMultiplierView } from './key-multiplier.view';

export function KeyGroupName({ rate }: { rate: KeyRateCache | null }) {
  return <span className="cell-hint block break-words">分组：{rate?.group_name || rate?.group || '未知'}</span>;
}

export function ProviderKeyRates({ keys }: { keys: KeyStatus[] }) {
  return <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-label="全部 Key 倍率">
    {keys.map(key => <div key={key.id} className="min-w-0 rounded-md border p-3 [overflow-wrap:anywhere]">
      <div className="flex flex-wrap items-baseline gap-x-2"><strong className="mono">{key.id}</strong>{!key.enabled && <span className="cell-hint">已停用</span>}</div>
      <KeyGroupName rate={key.rate} />
      <KeyMultiplierView value={key} />
    </div>)}
  </div>;
}

export function GroupRateObservation({ value }: { value: AccountRateObservation | null }) {
  if (!value) return <p className="card-hint">分组倍率 · 未查询</p>;
  const r = value.rate;
  return <div className="group-rate-observation">
    {r && <p className="group-rate-value">{r.group ? <>分组 {r.group_name || r.group}{r.group_name && `（${r.group}）`} · 倍率 </> : '当前计费倍率 '}<strong>{r.multiplier === null ? '未知' : `${r.multiplier}×`}</strong></p>}
    {value.failure ? <p className="feedback error-feedback" role="alert">{rateFailureText[value.failure] ?? '查询失败，分组倍率未知。'}{value.http_status > 0 && `（HTTP ${value.http_status}）`}</p>
      : r && <p className="card-hint">来源：{rateBasisText[r.basis]}</p>}
    <p className="card-hint">Key {value.key_id === 'anonymous' ? '匿名' : value.key_id}{value.remote_key_id > 0 && ` · 后台 ID ${value.remote_key_id}`} · {new Date(value.observed_at).toLocaleString()}</p>
  </div>;
}
