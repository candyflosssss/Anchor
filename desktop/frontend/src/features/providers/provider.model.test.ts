import { actionFor } from './provider-features.model';
import { describe, expect, it } from 'vitest';
import { isDirty, newDraft, toSave, validateDraft, type Provider } from './provider.model';
import { MockProviderTransport } from './provider.mock';
const source: Provider = { cost_multiplier: 1, id: '摸鱼AI', homepage_url: 'https://www.example.com', base_urls: ['https://api.example.com/v1', 'https://api2.example.com/v1'], enabled: false, timeout_seconds: 90, max_concurrent: 2, revision: 4, has_secret: true };
const valid = () => ({ ...newDraft(), id: '新供应商', base_urls: ['https://api.example.com/v1'] });
const signal = () => new AbortController().signal;
describe('draft conversion aligned to current provider DTO', () => {
  it('uses real defaults and separates saved data from draft', () => {
    expect(newDraft().timeout).toBe('120'); expect(newDraft().concurrent).toBe('4');
    const draft = newDraft(source); draft.timeout = '180';
    expect(source.timeout_seconds).toBe(90); expect(isDirty(draft, source)).toBe(true);
    expect(isDirty(newDraft(source), source)).toBe(false);
  });
  it('preserves every ordinary field in a complete replacement, including disabled', () => {
    const saved = toSave(newDraft(source), source);
    expect(saved.provider).toEqual(source); expect(saved.expected_revision).toBe(4); expect(saved.secret).toBeNull();
  });
  it('maps create / replace / clear / keep without masked placeholders', () => {
    expect(toSave(valid()).expected_revision).toBe(0);
    expect(toSave(valid()).secret).toBeNull();
    expect(toSave({ ...valid(), secret: '  synthetic-create-key  ' }).secret).toBe('synthetic-create-key');
    expect(toSave({ ...newDraft(source), secretMode: 'replace', secret: '  test-only  ' }, source).secret).toBe('test-only');
    expect(toSave({ ...newDraft(source), secretMode: 'clear', confirmClear: true }, source).secret).toBe('');
    expect(toSave(newDraft(source), source).secret).toBeNull();
  });
  it('rejects revision values which cannot be represented safely', () => {
    expect(() => toSave(newDraft(source), { ...source, revision: Number.MAX_SAFE_INTEGER + 1 })).toThrow();
  });
});
describe('immediate field feedback, not replacement backend validation', () => {
  it.each(['摸鱼AI', '供应商_2', 'équipe-1', 'Ａ公司', 'abc.123', '𠀀服务'])('supports valid Unicode ID %s', id => {
    expect(validateDraft({ ...valid(), id }).id).toBeUndefined();
  });
  it.each(['', '含 空格', '-start', 'emoji😀', 'a'.repeat(129), '\ud800'])('rejects invalid ID %s', id => {
    expect(validateDraft({ ...valid(), id }).id).toBeDefined();
  });
  it.each(['https://api.example.com/v1', 'http://127.0.0.1:18080/v1', 'http://[::1]:18080/v1', 'http://localhost/v1', 'http://localhost:18080/v1', 'http://LOCALHOST:18080/v1'])('allows base URL %s', base_url => {
    expect(validateDraft({ ...valid(), base_urls: [base_url] }).base_url_0).toBeUndefined();
  });
  it.each(['http://localhost.example/v1', 'http://sub.localhost/v1', 'http://localhost./v1', 'http://localhost@outside.example/v1', 'http://127.1/v1', 'http://example.com', 'https://a:b@example.com', 'https://example.com?q=1', 'https://example.com#key', 'https://example.com/v1/responses', 'https://example.com\\foo'])('rejects URL %s', base_url => {
    expect(validateDraft({ ...valid(), base_urls: ['https://ok.example.com', base_url] }).base_url_1).toBeDefined();
  });
  it('checks multiple URLs for duplicates and count, and homepage separately', () => {
    expect(validateDraft({ ...valid(), base_urls: ['https://a.example.com/v1', 'https://a.example.com/v1/'] }).base_url_1).toBe('请求地址重复。');
    expect(validateDraft({ ...valid(), base_urls: [] }).base_urls).toBeDefined();
    expect(validateDraft({ ...valid(), base_urls: Array.from({ length: 17 }, (_, i) => `https://a${i}.example.com`) }).base_urls).toBeDefined();
    expect(validateDraft({ ...valid(), homepage_url: '' }).homepage_url).toBeUndefined();
    expect(validateDraft({ ...valid(), homepage_url: 'http://www.example.com' }).homepage_url).toBeUndefined();
    expect(validateDraft({ ...valid(), homepage_url: 'ftp://example.com' }).homepage_url).toBeDefined();
  });
  it('creates providers with an optional Key and the chosen request protocol', () => {
    const saved = toSave({ ...valid(), secretMode: 'replace', secret: 'synthetic-create-key' });
    expect(saved.secret).toBe('synthetic-create-key'); expect(saved.provider.protocol).toBe('responses');
    expect(saved.provider.base_urls).toEqual(['https://api.example.com/v1']);
  });
  it('checks timeout and concurrency limits including blank and decimals', () => {
    for (const value of ['', '0', '3601', '1.5']) expect(validateDraft({ ...valid(), timeout: value }).timeout).toBeDefined();
    for (const value of ['', '0', '65', '1.5']) expect(validateDraft({ ...valid(), concurrent: value }).concurrent).toBeDefined();
    expect(validateDraft({ ...valid(), timeout: '3600', concurrent: '64' })).toEqual({});
  });
  it('requires replacement and clear confirmation', () => {
    expect(validateDraft(valid())).toEqual({});
    expect(validateDraft({ ...newDraft(source), secretMode: 'replace' }).secret).toBeDefined();
    expect(validateDraft({ ...newDraft(source), secretMode: 'clear' }).secret).toBeDefined();
    expect(validateDraft({ ...newDraft(source), secretMode: 'clear', confirmClear: true }).secret).toBeUndefined();
  });
  it('checks secret in UTF-8 bytes, without logging its value', () => {
    const replace = { ...newDraft(source), secretMode: 'replace' as const };
    expect(validateDraft({ ...replace, secret: '密'.repeat(2731) }).secret).toBeDefined();
    expect(validateDraft({ ...replace, secret: 'test\nvalue' }).secret).toBeDefined();
    expect(validateDraft({ ...replace, secret: '  test-only  ' }).secret).toBeUndefined();
  });
});
describe('mock is isolated, revision-aware and never retains secrets', () => {
  it('does not expose secrets in list or saved metadata', async () => {
    const mock = new MockProviderTransport();
    const created = await mock.save(toSave(valid()), signal());
    expect(created.has_secret).toBe(false);
    const value = await mock.save(toSave({ ...newDraft(created), secretMode: 'replace', secret: 'unit-test-only' }, created), signal());
    expect(value.has_secret).toBe(true); expect(JSON.stringify(mock)).not.toContain('unit-test-only');
    expect(JSON.stringify(await mock.list(signal()))).not.toContain('unit-test-only');
    await mock.save(toSave({ ...newDraft(value), secretMode: 'clear', confirmClear: true }, value), signal());
    expect((await mock.list(signal())).find(p => p.id === value.id)?.has_secret).toBe(false);
  });
  it('protects duplicate creation and stale revision', async () => {
    const mock = new MockProviderTransport();
    const value = await mock.save(toSave(valid()), signal());
    await expect(mock.save(toSave(valid()), signal())).rejects.toMatchObject({ code: 'conflict' });
    await mock.save(toSave(newDraft(value), value), signal());
    await expect(mock.save(toSave(newDraft(value), value), signal())).rejects.toMatchObject({ code: 'conflict' });
  });
  it('makes unknown writes observable only by a later explicit read', async () => {
    const mock = new MockProviderTransport(); mock.reset('unknown');
    await expect(mock.save(toSave(valid()), signal())).rejects.toMatchObject({ code: 'unknown' });
    expect((await mock.list(signal())).find(p => p.id === '新供应商')?.revision).toBe(1);
  });
  it('aborted reads cannot mutate the view', async () => {
    const mock = new MockProviderTransport(); const abort = new AbortController();
    const result = mock.list(abort.signal); abort.abort();
    await expect(result).rejects.toMatchObject({ name: 'AbortError' });
  });
});


describe('optional creation Key', () => {
  it('validates entered Keys even without the edit-only replace mode', () => {
    expect(validateDraft({ ...valid(), secret: 'synthetic-create-key' })).toEqual({});
    expect(validateDraft({ ...valid(), secret: 'first\nsecond' }).secret).toBeDefined();
    expect(validateDraft({ ...valid(), secret: 'x'.repeat(8193) }).secret).toBeDefined();
    expect(validateDraft({ ...valid(), secret: '密'.repeat(2731) }).secret).toBeDefined();
  });
});

describe('provider recharge cost factor', () => {
  it('defaults to 1 and saves a 1:2 recharge as 0.5', () => {
    expect(newDraft().costMultiplier).toBe('1');
    const draft = { ...valid(), costMultiplier: '0.5' };
    expect(validateDraft(draft)).toEqual({});
    expect(toSave(draft).provider.cost_multiplier).toBe(0.5);
    expect(newDraft({ ...source, cost_multiplier: 0.5 }).costMultiplier).toBe('0.5');
  });
  it.each(['', '0', '-1', 'NaN', 'Infinity', '1001', '0.0000001'])('rejects invalid factor %s', costMultiplier => {
    expect(validateDraft({ ...valid(), costMultiplier }).costMultiplier).toBeDefined();
  });
});

describe('provider request protocol editing', () => {
  it('sends an explicit protocol only for creation or a changed selection', () => {
    const p = { ...source, protocol: 'anthropic' };
    expect(newDraft(p).protocol).toBe('anthropic');
    expect(toSave(newDraft(p), p).provider).not.toHaveProperty('protocol');
    expect(toSave({ ...newDraft(p), protocol: 'chat-completions' }, p).provider.protocol).toBe('chat-completions');
    expect(newDraft(source).protocol).toBe('');
    expect(toSave(newDraft(source), source).provider).not.toHaveProperty('protocol');
  });
  it.each(['unknown', 'toString', '__proto__'])('rejects unsupported request protocol %s', protocol => {
    expect(validateDraft({ ...valid(), protocol }).protocol).toBeDefined();
  });
  it('persists changed protocols in the prototype and preserves named Keys and model overrides', async () => {
    const transport = new MockProviderTransport();
    let p = (await transport.list(signal()))[0];
    let w = await transport.action(actionFor(p, 'key-put', { target: 'named', secret: 'synthetic-protocol', protocol: 'responses', enabled: true }), signal());
    await transport.action(actionFor({ ...p, revision: w.revision }, 'key-model-protocol', { target: 'default', model: 'override-model', protocol: 'chat-completions' }), signal());
    p = (await transport.list(signal())).find(row => row.id === p.id)!;
    p = await transport.save(toSave({ ...newDraft(p), protocol: 'anthropic' }, p), signal());
    w = await transport.workspace(p.id, signal());
    expect(p.protocol).toBe('anthropic');
    expect(w.keys.find(k => k.id === 'default')).toMatchObject({ protocol: 'anthropic', model_protocols: [{ model: 'override-model', protocol: 'chat-completions' }] });
    expect(w.keys.find(k => k.id === 'named')?.protocol).toBe('responses');
    await transport.action(actionFor(p, 'key-put', { target: 'default', protocol: 'responses', enabled: true }), signal());
    p = (await transport.list(signal())).find(row => row.id === p.id)!;
    await transport.save(toSave({ ...newDraft(p), timeout: '240' }, p), signal());
    expect((await transport.workspace(p.id, signal())).keys.find(k => k.id === 'default')?.protocol).toBe('responses');
  });
});
