import { useState, type FormEvent } from 'react';
import { RefreshCw, LoaderCircle } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { requestURLError } from './provider.model';
import { accountFailureText, amountText, platformLabels, allowanceScopeText, billingScopeHint, type AccountObservation, type AccountSettings, type AccountQuery } from './provider-account.model';
import { GroupRateObservation } from './provider-rate.view';
import type { ProviderController } from './use-provider-controller';

export function ProviderAccountView({ c }: { c: ProviderController }) {
  const a = c.account;
  return <div className="stack">
    {a.error && <div className="feedback error-feedback" role="alert"><span>{a.error}</span><Button size="sm" variant="outline" disabled={c.busy} onClick={() => void c.load()}>重新读取配置</Button></div>}
    {!a.settings ? <div className="empty-state compact" role="status">{a.busy ? <><LoaderCircle className="loading-icon" size={24} /><p>正在读取平台配置…</p></> : <p>请重新读取平台配置后操作。</p>}</div>
      : <AccountContent key={`${a.settings.provider_id}:${a.settings.revision}`} c={c} settings={a.settings} />}
  </div>;
}
function AccountContent({ c, settings: s }: { c: ProviderController; settings: AccountSettings }) {
  const [platform, setPlatform] = useState(s.platform || 'auto');
  const [siteURL, setSiteURL] = useState(s.site_url);
  const [userID, setUserID] = useState(s.user_id ? String(s.user_id) : '');
  const [tokenMode, setTokenMode] = useState<'keep' | 'replace' | 'clear'>('keep');
  const [token, setToken] = useState('');
  const [confirmClear, setConfirmClear] = useState(false);
  const [formError, setFormError] = useState('');
  const [keyID, setKeyID] = useState('');
  const [remoteID, setRemoteID] = useState('');
  const a = c.account;
  const dirty = platform !== (s.platform || 'auto') || siteURL.trim().replace(/\/+$/, '') !== s.site_url || (userID.trim() || '0') !== String(s.user_id) || tokenMode !== 'keep';
  const disabled = c.blocked || a.needsRead;
  const queryDisabled = disabled || dirty;
  const authTargetChanged = platform !== (s.platform || 'auto') || siteURL.trim().replace(/\/+$/, '') !== s.site_url || Number(userID || '0') !== s.user_id;
  const keys = c.features.data?.keys ?? [];
  const oneAPI = s.platform === 'oneapi' || a.observedPlatform === 'oneapi';
  const newAPI = platform === 'newapi' || platform === 'auto' && a.observedPlatform === 'newapi';
  const tokenLabel = newAPI ? 'NewAPI 系统访问令牌' : '用户令牌';
  async function save(e: FormEvent) {
    e.preventDefault();
    if (disabled) return;
    setFormError('');
    if (!platformLabels[platform]) { setFormError('请先选择平台类型。'); return; }
    if (siteURL.trim() && requestURLError(siteURL.trim())) { setFormError(requestURLError(siteURL.trim()) ?? '地址无效'); return; }
    const uid = userID.trim() || '0';
    if (!/^\d+$/.test(uid) || !Number.isSafeInteger(Number(uid))) { setFormError('用户 ID 必须为安全范围内的非负整数。'); return; }
    if (tokenMode === 'replace' && (!token.trim() || /[\r\n]/.test(token) || new TextEncoder().encode(token.trim()).length > 8192)) { setFormError('用户令牌须非空、无换行，且不超过 8192 字节。'); return; }
    if (tokenMode === 'clear' && !confirmClear) { setFormError('请确认清除已保存用户令牌。'); return; }
    const userToken = tokenMode === 'keep' ? null : tokenMode === 'clear' ? '' : token.trim();
    setToken(''); // Never keep a submitted credential in the visible form, including failures.
    await a.save({ provider_id: s.provider_id, expected_revision: s.revision, platform, site_url: siteURL.trim(), user_id: Number(uid), user_token: userToken });
  }
  function query(operation: 'balance' | 'key-quota' | 'effective-rate') {
    if (queryDisabled) return;
    setFormError('');
    let remote = 0;
    if (operation === 'key-quota' && oneAPI) {
      if (!/^\d+$/.test(remoteID) || !Number.isSafeInteger(+remoteID) || +remoteID < 1) { setFormError('请填写 OneAPI 后台中该令牌的正整数 ID；它不是 Anchor 的本地 Key 名称。'); return; }
      remote = +remoteID;
    }
    if (operation === 'effective-rate' && remoteID.trim()) {
      if (!/^\d+$/.test(remoteID.trim()) || !Number.isSafeInteger(+remoteID) || +remoteID < 1) { setFormError('后台 Key ID 必须为安全范围内的正整数；留空可自动核对所选 Key。'); return; }
      remote = +remoteID;
    }
    const input: AccountQuery = { provider_id: s.provider_id, expected_revision: s.revision, operation, key_id: keyID, remote_key_id: remote };
    void a.query(input);
  }
  return <>
    <section className="detail-card">
      <h2>平台账户配置</h2>
      <form className="account-form" onSubmit={e => void save(e)} autoComplete="off">
        <fieldset disabled={disabled} className="account-fields">
          <div className="field"><label htmlFor="account-platform">平台类型</label><select id="account-platform" className="select-field" value={platform} onChange={e => setPlatform(e.target.value)}>{Object.entries(platformLabels).map(([id, label]) => <option value={id} key={id}>{label}</option>)}</select></div>
          {newAPI && <div className="field"><label htmlFor="account-user-id">NewAPI 用户 ID（按站点要求填写）</label><input id="account-user-id" inputMode="numeric" value={userID} onChange={e => setUserID(e.target.value)} placeholder="可先留空；该站要求时填写数字用户 ID" /><p className="card-hint">这是该站后台的账户 ID。仅当该站要求用户 ID 时填写。</p></div>}
          <details className="pricing-details"><summary>账户接口地址（可选）</summary><div className="field"><label htmlFor="account-site-url">站点根地址</label><input id="account-site-url" value={siteURL} onChange={e => setSiteURL(e.target.value)} placeholder="留空，沿用请求地址并移除末尾 /v1" /><p className="card-hint">后台与推理 API 地址不同时填写。包含站点实际使用的部署路径前缀。</p></div></details>
          {(platform === 'sub2api' || a.observedPlatform === 'sub2api') && <p className="card-hint">sub2api 查询所选 Key 的当前计费倍率可直接使用推理 Key，无需用户 JWT。旧站缺少 Key 倍率接口，或指定后台 Key ID 时，才需要用户授权。</p>}
          {platform === 'auto' && !a.observedPlatform && <p className="card-hint">NewAPI 账户余额需要后台单独生成的系统访问令牌。选择「NewAPI」可查看获取步骤并配置。</p>}
          {newAPI && <div className="card-hint">
            <p>登录该 NewAPI 站点后台 → 个人设置／安全设置 → 系统访问令牌，生成后选择下方「输入／替换系统访问令牌」，粘贴并保存。它与调用模型的 API Key 分开配置。</p>
            <p>支持权限选择的站点：余额查询需「个人信息读取」（profile:read）；自动核对 Key 的分组倍率还需「API Key 读取」（api_key:read）和「查看完整 API Key」（api_key:reveal）。</p>
            <p>{s.has_user_token ? '已保存系统访问令牌，列表与详情优先查询账户余额。' : '尚未配置系统访问令牌，查询结果可能是 Key 限额；确认账户余额请先配置。'}</p>
          </div>}
          <div className="field"><label htmlFor="account-token-mode">{tokenLabel} · {s.has_user_token ? '已保存' : '未配置'}</label><select id="account-token-mode" className="select-field" value={tokenMode} onChange={e => { setTokenMode(e.target.value as typeof tokenMode); setToken(''); setConfirmClear(false); }}><option value="keep">保留现有授权</option><option value="replace">输入／替换{newAPI ? '系统访问令牌' : '用户令牌'}</option><option value="clear">清除{newAPI ? '系统访问令牌' : '用户令牌'}</option></select>
            {tokenMode === 'replace' && <><label htmlFor="account-token" className="account-token-label">{newAPI ? tokenLabel : '平台用户令牌'}</label><input id="account-token" type="password" autoComplete="new-password" value={token} onChange={e => setToken(e.target.value)} placeholder={newAPI ? '粘贴后台生成的系统访问令牌' : platform === 'sub2api' ? '用户登录 JWT' : '平台用户 access token'} /></>}
            {tokenMode === 'clear' && <label className="account-clear"><input type="checkbox" checked={confirmClear} onChange={e => setConfirmClear(e.target.checked)} />确认清除，之后部分查询将需要重新授权</label>}
            {s.has_user_token && authTargetChanged && tokenMode === 'keep' && <p className="card-hint">平台、站点地址或用户 ID 已改变，保存时将清除旧用户令牌。需要继续查询时请选择「输入／替换用户令牌」。</p>}
          </div>
        </fieldset>
        <Button type="submit" disabled={disabled || !platform || !dirty}>{a.busy ? '正在处理…' : '保存平台配置'}</Button>
      </form>
      {dirty && <p className="card-hint">草稿尚未保存。</p>}
      {formError && <p className="feedback error-feedback" role="alert">{formError}</p>}
    </section>
    <section className="detail-card">
      <div className="card-toolbar"><h2>查询使用的 Key</h2><select className="select-field" aria-label="余额查询 Key" disabled={disabled} value={keyID} onChange={e => { setKeyID(e.target.value); setRemoteID(''); a.clearObservations(); }}><option value="">默认 Key（优先 default）</option>{keys.map(k => <option key={k.id} value={k.id}>{k.id}{k.enabled ? '' : '（已停用）'}</option>)}</select></div>
      {!oneAPI && <div className="field account-remote"><label htmlFor="account-rate-key-id">后台 Key ID（查询分组倍率时可选）</label><input id="account-rate-key-id" inputMode="numeric" value={remoteID} disabled={disabled} onChange={e => { setRemoteID(e.target.value); a.clearObservations(); }} placeholder="留空，自动核对所选 Key" /><p className="card-hint">留空时查询所选 Key，sub2api 优先直接读取。填写 ID 时需要用户授权，并查询该远端 Key 的分组；请在后台核对 Key 对应关系。</p></div>}
      {oneAPI && <div className="field account-remote"><label htmlFor="account-remote-id">OneAPI 后台令牌 ID（查询 Key 额度时必填）</label><input id="account-remote-id" inputMode="numeric" value={remoteID} disabled={disabled} onChange={e => { setRemoteID(e.target.value); a.clearObservations(); }} placeholder="例如 123；请在 OneAPI 后台核对" /></div>}
    </section>
    <section className="detail-card">
      <div className="card-toolbar"><h2>分组倍率</h2><Button variant="outline" size="sm" disabled={queryDisabled} onClick={() => query('effective-rate')}><RefreshCw size={14} />刷新分组倍率</Button></div>
      <GroupRateObservation value={a.rate} />
      <p className="card-hint">这是平台返回的倍率快照。sub2api Key 直读显示含个人覆盖和高峰规则的当前计费倍率，未返回的分组名称留空。模型映射中的路由倍率单独保存，查询不会修改路由。</p>
    </section>
    <div className="overview-grid">
      <Observation title="余额" value={a.balance} disabled={queryDisabled} onRefresh={() => query('balance')} />
      <Observation title="Key 额度" value={a.quota} disabled={queryDisabled} onRefresh={() => query('key-quota')} />
    </div>
  </>;
}
function Observation({ title, value, disabled, onRefresh }: { title: string; value: AccountObservation | null; disabled: boolean; onRefresh: () => void }) {
  const a = value?.allowance;
  return <section className="detail-card">
    <div className="card-toolbar"><h2>{title}</h2><Button variant="outline" size="sm" disabled={disabled} onClick={onRefresh}><RefreshCw size={14} />刷新{title === '余额' ? '余额' : '额度'}</Button></div>
    {!value ? <p className="card-hint">尚未查询。</p> : value.failure ? <p className="feedback error-feedback" role="alert">{accountFailureText[value.failure] ?? '查询失败，平台暂未返回可用数据。'}{value.http_status > 0 ? `（HTTP ${value.http_status}）` : ''}</p> : a && <>
      <p className="card-hint">{allowanceScopeText(a.scope)}{a.scope === 'billing' ? ` · ${billingScopeHint}` : ''}</p>
      <p className="account-amount">{a.unlimited === true ? '无限额' : amountText(a.remaining)}</p>
      <dl className="account-numbers"><div><dt>已用</dt><dd>{amountText(a.used)}</dd></div><div><dt>总额度</dt><dd>{amountText(a.limit)}</dd></div></dl>
      {a.windows.map((w, i) => <div className="account-window" key={`${w.period}:${i}`}><strong>{({ daily: '每日', weekly: '每周', monthly: '每月', '5h': '5 小时', '1d': '每日', '7d': '7 天' } as Record<string, string>)[w.period] ?? w.period}窗口</strong><p>剩余 {amountText(w.remaining)} · 已用 {amountText(w.used)} · 限额 {amountText(w.limit)}</p>{w.reset_at && <p className="card-hint">重置时间：{w.reset_at}</p>}</div>)}
    </>}
    {value && <p className="card-hint">查询于 {new Date(value.observed_at).toLocaleString()} · Key {value.key_id === 'anonymous' ? '匿名' : value.key_id}{value.remote_key_id > 0 ? ` · 后台令牌 ID ${value.remote_key_id}` : ''}</p>}
  </section>;
}
