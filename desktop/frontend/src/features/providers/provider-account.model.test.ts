import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-account.json';
import { amountText, allowanceScopeText, checkedAccountObservation, checkedAccountSettings, checkedAccountRate } from './provider-account.model';
import { DesktopProviderTransport } from '../../desktop/transport';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
const balanceQuery = { provider_id: 'demo', expected_revision: 3, operation: 'balance', key_id: '', remote_key_id: 0 };
const signal = () => new AbortController().signal;

describe('account contract and precision', () => {
  it('uses shared public settings, decimal strings, unknown fields and a separate unlimited key', () => {
    expect(checkedAccountSettings({ ...fixture.settings, user_token: 'synthetic-token' } as typeof fixture.settings, 'demo')).toEqual(fixture.settings);
    const v = checkedAccountObservation(fixture.balance, balanceQuery);
    expect(amountText(v.allowance!.remaining)).toBe('12.35 USD');
    expect(amountText(v.allowance!.used)).toBe('未知');
    const q = checkedAccountObservation(fixture.quota, { ...balanceQuery, operation: 'key-quota' });
    expect(q.allowance!.unlimited).toBe(true);
    expect(q.allowance!.remaining).toBeNull();
    expect(amountText({ value: '9007199254740993123', unit: 'quota' })).toBe('9007199254740993123.00 quota（原始额度）');
  });
  it('accepts automatic settings and scoped default-key summaries', () => {
    expect(checkedAccountSettings({ ...fixture.settings, platform: 'auto' }, 'demo').platform).toBe('auto');
    const q = { ...balanceQuery, operation: 'balance-summary' };
    expect(checkedAccountObservation(fixture.summary, q).allowance!.scope).toBe('account');
    expect(checkedAccountObservation(fixture.key_summary, q).allowance!.scope).toBe('key');
    expect(() => checkedAccountObservation({ ...fixture.key_summary, allowance: { ...fixture.key_summary.allowance, scope: 'subscription' } }, q)).toThrow('read_error');
  });
  it('formats amounts with two decimals without rounding the stored observation', () => {
    for (const [value, expected] of [
      ['1.005', '1.01'], ['-1.005', '-1.01'], ['9.999', '10.00'],
      ['0', '0.00'], ['-0.004', '0.00'], ['1e-7', '0.00'],
      ['1.2345e2', '123.45'], ['5e-3', '0.01'], ['5e-4', '0.00'],
      ['9007199254740993123.455', '9007199254740993123.46'],
    ]) expect(amountText({ value, unit: 'USD' })).toBe(`${expected} USD`);
    expect(checkedAccountObservation(fixture.balance, balanceQuery).allowance!.remaining!.value).toBe('12.34567890123456789');
  });
  it('accepts NewAPI billing for balances and summaries, preserving unknown currencies', () => {
    expect(checkedAccountObservation(fixture.billing, balanceQuery)).toEqual(fixture.billing);
    expect(checkedAccountObservation(fixture.billing_summary, { ...balanceQuery, operation: 'balance-summary' })).toEqual(fixture.billing_summary);
    expect(amountText({ value: '7.3', unit: 'CNY' })).toBe('7.30 CNY');
    expect(amountText({ value: '7.3', unit: 'site_unit' })).toContain('币种未确认');
    expect(allowanceScopeText('billing')).toBe('站点余额');
    expect(() => checkedAccountObservation({ ...fixture.billing, platform: 'sub2api' }, balanceQuery)).toThrow('read_error');
    expect(() => checkedAccountObservation({ ...fixture.billing, operation: 'key-quota' }, { ...balanceQuery, operation: 'key-quota' })).toThrow('read_error');
  });
  it('rejects wrong provider, revision, selected key, scope and malformed or missing amounts', () => {
    for (const changes of [ { provider_id: 'other' }, { revision: 4 }, { remote_key_id: 3 },
      { allowance: { ...fixture.balance.allowance, scope: 'key' } },
      { allowance: { ...fixture.balance.allowance, remaining: { value: 'NaN', unit: 'USD' } } },
      { failure: 'auth_required' },
    ]) expect(() => checkedAccountObservation({ ...fixture.balance, ...changes }, balanceQuery)).toThrow('read_error');
    expect(() => checkedAccountObservation({ ...fixture.balance, allowance: { ...fixture.balance.allowance, used: undefined } } as unknown as typeof fixture.balance, balanceQuery)).toThrow('read_error');
    expect(() => checkedAccountObservation(fixture.balance, { ...balanceQuery, key_id: 'another' })).toThrow('read_error');
    expect(() => checkedAccountSettings({ ...fixture.settings, user_id: Number.MAX_SAFE_INTEGER + 1 }, 'demo')).toThrow('read_error');
  });
  it('keeps failed queries free of fabricated zero balances', () => {
    const v = checkedAccountObservation({ ...fixture.balance, failure: 'auth_required', allowance: null }, balanceQuery);
    expect(v.allowance).toBeNull();
    expect(v.failure).toBe('auth_required');
  });
});

describe('account desktop transport', () => {
  it('maps each operation to its typed bridge and strips unexpected secrets', async () => {
    const input = { provider_id: 'demo', expected_revision: 2, platform: 'newapi', site_url: '', user_id: 7, user_token: 'synthetic-private-user-token' };
    let saved: unknown;
    const bridge = {
      ProviderAccountSettings: async () => ({ contract: CONTRACT, data: { ...fixture.settings, user_token: input.user_token }, problem: null }),
      SaveProviderAccount: async (v: typeof input) => { saved = { ...v }; return { contract: CONTRACT, data: fixture.settings, problem: null }; },
      ProviderAllowance: async () => ({ contract: CONTRACT, data: fixture.balance, problem: null }),
    } as unknown as DesktopBridge;
    const t = new DesktopProviderTransport(() => bridge);
    expect(await t.accountSettings('demo', signal())).toEqual(fixture.settings);
    expect(await t.saveAccount(input, signal())).toEqual(fixture.settings);
    expect(saved).toEqual(input);
    expect(await t.allowance(balanceQuery, signal())).toEqual(fixture.balance);
  });
  it('distinguishes read failures, write outcome unknown and conflicts', async () => {
    const broken = { ProviderAccountSettings: async () => { throw Error('synthetic-network-error'); }, SaveProviderAccount: async () => { throw Error('synthetic-network-error'); },
      ProviderAllowance: async () => ({ contract: CONTRACT, data: null, problem: { code: 'conflict' } }) } as unknown as DesktopBridge;
    const t = new DesktopProviderTransport(() => broken);
    await expect(t.accountSettings('demo', signal())).rejects.toThrow('offline');
    await expect(t.saveAccount({ provider_id: 'demo', expected_revision: 3, platform: 'sub2api', site_url: '', user_id: 0, user_token: null }, signal())).rejects.toThrow('unknown');
    await expect(t.allowance(balanceQuery, signal())).rejects.toThrow('conflict');
  });
  it('discarded bridge reads cannot return data after cancellation', async () => {
    const abort = new AbortController();
    const t = new DesktopProviderTransport(() => ({ ProviderAllowance: async () => { abort.abort(); return { contract: CONTRACT, data: fixture.balance, problem: null }; } } as unknown as DesktopBridge));
    await expect(t.allowance(balanceQuery, abort.signal)).rejects.toThrow();
  });
});

describe('group rate contract', () => {
  const query = { provider_id: 'demo', expected_revision: 3, operation: 'effective-rate', key_id: 'default', remote_key_id: 0 };
  it('preserves precision and accepts unknown auto groups without inventing 1', () => {
    expect(checkedAccountRate(fixture.rate, query)).toEqual(fixture.rate);
    const auto = { ...fixture.rate, platform: 'newapi', failure: 'unknown', rate: { ...fixture.rate.rate, group: 'auto', multiplier: null } };
    expect(checkedAccountRate(auto, query).rate?.multiplier).toBeNull();
    expect(checkedAccountRate({ ...auto, rate: null }, query).rate).toBeNull();
  });
  it('rejects stale, malformed, negative, missing and failed positive observations', () => {
    for (const patch of [{ revision: 2 }, { key_id: 'other' }, { operation: 'balance' }, { rate: null },
      { rate: { ...fixture.rate.rate, multiplier: '-1' } }, { rate: { ...fixture.rate.rate, multiplier: 'NaN' } },
      { failure: 'authentication' }, { rate: { ...fixture.rate.rate, basis: 'guess' } }]) {
      expect(() => checkedAccountRate({ ...fixture.rate, ...patch }, query)).toThrow('read_error');
    }
  });
  it('accepts key-only sub2api billing without fabricating a group', () => {
    expect(checkedAccountRate(fixture.key_billing_rate, query)).toEqual(fixture.key_billing_rate);
    for (const patch of [{ platform: 'newapi' }, { remote_key_id: 7 },
      { rate: { ...fixture.key_billing_rate.rate, basis: 'authenticated_group' } }]) {
      const observation = { ...fixture.key_billing_rate, ...patch };
      expect(() => checkedAccountRate(observation, { ...query, remote_key_id: observation.remote_key_id })).toThrow('read_error');
    }
  });
  it('uses only the rate bridge and drops unexpected response properties', async () => {
    const t = new DesktopProviderTransport(() => ({ ProviderRate: async () => ({ contract: CONTRACT, data: { ...fixture.rate, secret: 'discard', rate: { ...fixture.rate.rate, secret: 'discard' } }, problem: null }) } as unknown as DesktopBridge));
    expect(await t.rate(query, signal())).toEqual(fixture.rate);
  });
});
