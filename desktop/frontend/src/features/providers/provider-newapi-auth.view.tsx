import { useState } from 'react';
import { ImeForm } from '../../components/shared/ime-form';
import { Button } from '../../components/ui/button';
import { rateFailureText, type AccountSettings } from './provider-account.model';
import { GroupRateObservation, ProviderKeyRates } from './provider-rate.view';
import type { ProviderController } from './use-provider-controller';

/** The parent keys this form by provider and revision; the workbench isolates transports. */
export function ProviderNewAPIAuth({ c }: { c: ProviderController }) {
  const a = c.account, s = a.settings;
  if (!s || (s.platform !== 'newapi' && a.observedPlatform !== 'newapi')) return null;
  return <NewAPIAuthForm key={`${s.provider_id}:${s.revision}:${s.user_id}`} c={c} settings={s} />;
}
function NewAPIAuthForm({ c, settings: s }: { c: ProviderController; settings: AccountSettings }) {
  const a = c.account;
  const workspace = c.features.data;
  const w = workspace?.provider_id === s.provider_id && workspace.revision === s.revision ? workspace : null;
  const [token, setToken] = useState('');
  const [userID, setUserID] = useState(s.user_id ? String(s.user_id) : '');
  const failure = a.rate?.failure || a.balance?.failure || '';
  const needsToken = !s.has_user_token || ['auth_required', 'authentication', 'permission_denied'].includes(failure);
  return <section className="detail-card" aria-label="NewAPI 用户授权">
    {needsToken ? <>
      <h2>已识别 NewAPI · 用户令牌</h2>
      <p className="card-hint">填入该站后台「个人设置 / 安全设置 → 系统访问令牌」，保存后自动查询分组倍率。</p>
      <ImeForm className="account-form" autoComplete="off" onSubmit={e => {
        e.preventDefault();
        if (c.blocked || a.needsRead || !a.ready) return;
        const value = token; setToken('');
        void a.authorizeNewAPI(value, userID);
      }}>
        <fieldset className="account-fields" disabled={c.blocked || a.needsRead || !a.ready}>
          <div className="field"><label htmlFor="newapi-user-token">NewAPI 用户令牌</label><input id="newapi-user-token" type="password" autoComplete="new-password" value={token} onChange={e => setToken(e.target.value)} placeholder="粘贴系统访问令牌" /></div>
          <details className="pricing-details"><summary>用户 ID 与令牌权限（按站点要求）</summary>
            <div className="field"><label htmlFor="newapi-user-id">NewAPI 用户 ID（可选）</label><input id="newapi-user-id" inputMode="numeric" value={userID} onChange={e => setUserID(e.target.value)} placeholder="通常留空；站点要求时填写" /></div>
            <p className="card-hint">令牌需有 API Key 读取与完整 Key 查看权限（api_key:read、api_key:reveal）；账户余额还需 profile:read。</p>
          </details>
        </fieldset>
        <Button type="submit" disabled={c.blocked || a.needsRead || !a.ready || !token.trim()}>{a.busy ? '正在处理…' : '保存并查询分组倍率'}</Button>
      </ImeForm>
    </> : <h2>NewAPI 分组倍率</h2>}
    {a.error && <div className="feedback error-feedback" role="alert"><span>{a.error}</span>{a.needsRead && <Button variant="outline" size="sm" disabled={c.busy} onClick={() => void c.load()}>重新读取配置</Button>}</div>}
    {w?.keys.length ? <>
      <ProviderKeyRates keys={w.keys} />
      {a.rate?.failure && a.rate.failure !== 'auth_required' && <p className="feedback error-feedback" role="alert">Key {a.rate.key_id}：{rateFailureText[a.rate.failure] ?? '查询失败，分组倍率未知。'}{a.rate.http_status > 0 && `（HTTP ${a.rate.http_status}）`}</p>}
    </> : w ? <GroupRateObservation value={a.rate} /> : <p className="card-hint" role="status">{c.features.error ? 'Key 倍率读取失败。' : '正在读取 Key 倍率…'}</p>}
  </section>;
}
