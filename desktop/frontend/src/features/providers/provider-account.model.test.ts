import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/provider-account.json';
import { amountText, checkedAccountObservation, checkedAccountSettings } from './provider-account.model';
import { DesktopProviderTransport } from '../../desktop/transport';
import { CONTRACT, type DesktopBridge } from '../../desktop/contract.generated';
const balanceQuery = { provider_id: 'demo', expected_revision: 3, operation: 'balance', key_id: '', remote_key_id: 0 };
const signal = () => new AbortController().signal;

describe('account contract and precision', () => {
  it('uses shared public settings, decimal strings, unknown fields and a separate unlimited key', () => {
    expect(checkedAccountSettings({ ...fixture.settings, user_token: 'synthetic-token' } as typeof fixture.settings, 'demo')).toEqual(fixture.settings);
    const v = checkedAccountObservation(fixture.balance, balanceQuery);
    expect(amountText(v.allowance!.remaining)).toBe('12.34567890123456789 USD');
    expect(amountText(v.allowance!.used)).toBe('未知');
    const q = checkedAccountObservation(fixture.quota, { ...balanceQuery, operation: 'key-quota' });
    expect(q.allowance!.unlimited).toBe(true);
    expect(q.allowance!.remaining).toBeNull();
    expect(amountText({ value: '9007199254740993123', unit: 'quota' })).toBe('9007199254740993123 quota（原始额度）');
  });
  it('accepts automatic settings and scoped default-key summaries', () => {
    expect(checkedAccountSettings({ ...fixture.settings, platform: 'auto' }, 'demo').platform).toBe('auto');
    const q = { ...balanceQuery, operation: 'balance-summary' };
    expect(checkedAccountObservation(fixture.summary, q).allowance!.scope).toBe('account');
    expect(checkedAccountObservation(fixture.key_summary, q).allowance!.scope).toBe('key');
    expect(() => checkedAccountObservation({ ...fixture.key_summary, allowance: { ...fixture.key_summary.allowance, scope: 'subscription' } }, q)).toThrow('read_error');
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
