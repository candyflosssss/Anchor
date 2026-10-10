import { describe, expect, it } from 'vitest';
import { isDirty, newDraft, toSave, validateDraft, type Provider } from './provider.model';
import { MockProviderTransport } from './provider.mock';
const source: Provider = { id: '摸鱼AI', base_url: 'https://api.example.com/v1', protocol: 'responses', enabled: false, timeout_seconds: 90, max_concurrent: 2, revision: 4, has_secret: true };
const valid = () => ({ ...newDraft(), id: '新供应商', base_url: 'https://api.example.com/v1' });
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
    expect(validateDraft({ ...valid(), id }, true).id).toBeUndefined();
  });
  it.each(['', '含 空格', '-start', 'emoji😀', 'a'.repeat(129), '\ud800'])('rejects invalid ID %s', id => {
    expect(validateDraft({ ...valid(), id }, true).id).toBeDefined();
  });
  it.each(['https://api.example.com/v1', 'http://127.0.0.1:18080/v1', 'http://[::1]:18080/v1'])('allows base URL %s', base_url => {
    expect(validateDraft({ ...valid(), base_url }, true).base_url).toBeUndefined();
  });
  it.each(['http://localhost/v1', 'http://127.1/v1', 'http://example.com', 'https://a:b@example.com', 'https://example.com?q=1', 'https://example.com#key', 'https://example.com/v1/responses', 'https://example.com\\foo'])('rejects URL %s', base_url => {
    expect(validateDraft({ ...valid(), base_url }, true).base_url).toBeDefined();
  });
  it('checks timeout and concurrency limits including blank and decimals', () => {
    for (const value of ['', '0', '3601', '1.5']) expect(validateDraft({ ...valid(), timeout: value }, true).timeout).toBeDefined();
    for (const value of ['', '0', '65', '1.5']) expect(validateDraft({ ...valid(), concurrent: value }, true).concurrent).toBeDefined();
    expect(validateDraft({ ...valid(), timeout: '3600', concurrent: '64' }, true)).toEqual({});
  });
  it('allows optional create secret; requires replacement and clear confirmation', () => {
    expect(validateDraft(valid(), true)).toEqual({});
    expect(validateDraft({ ...newDraft(source), secretMode: 'replace' }, false).secret).toBeDefined();
    expect(validateDraft({ ...newDraft(source), secretMode: 'clear' }, false).secret).toBeDefined();
    expect(validateDraft({ ...newDraft(source), secretMode: 'clear', confirmClear: true }, false).secret).toBeUndefined();
  });
  it('checks secret in UTF-8 bytes, without logging its value', () => {
    expect(validateDraft({ ...valid(), secret: '密'.repeat(2731) }, true).secret).toBeDefined();
    expect(validateDraft({ ...valid(), secret: 'test\nvalue' }, true).secret).toBeDefined();
    expect(validateDraft({ ...valid(), secret: '  test-only  ' }, true).secret).toBeUndefined();
  });
});
describe('mock is isolated, revision-aware and never retains secrets', () => {
  it('does not expose secrets in list or saved metadata', async () => {
    const mock = new MockProviderTransport();
    const value = await mock.save(toSave({ ...valid(), secret: 'unit-test-only' }), signal());
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
