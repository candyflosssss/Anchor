import { rateBasisText, rateFailureText, type AccountRateObservation } from './provider-account.model';

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
