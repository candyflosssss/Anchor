import { describe, expect, it } from 'vitest';
import { identifierPattern, isValidId } from './identifier';

describe('user-facing identifiers', () => {
  it('accepts Chinese names and counts Unicode code points', () => {
    for (const name of ['主力密钥', '备用_2.0', '中文模型', '开发服务器', '𠮷野家', '１２号', '鱼'.repeat(128), '𠮷'.repeat(128)]) expect(isValidId(name)).toBe(true);
    expect(isValidId('鱼'.repeat(129))).toBe(false);
    expect(isValidId('𠮷'.repeat(129))).toBe(false);
  });
  it('rejects unsafe names in both model and HTML form validation', () => {
    const formPattern = new RegExp(String.raw`^(?:${identifierPattern})(?![\s\S])`, 'v');
    for (const name of ['', '-中文', '_中文', '.中文', '中文 空格', '中文/路径', String.raw`中文\路径`, '中文:模型', '中文%2F路径', '中文?', '中文#', '中文\n', '中文\u0000', '中文\u200b', '中文\u202e', '中文\u0301', '中文😀']) {
      expect(isValidId(name), name).toBe(false);
      expect(formPattern.test(name), name).toBe(false);
    }
    expect(formPattern.test('开发服务器-2')).toBe(true);
  });
});
