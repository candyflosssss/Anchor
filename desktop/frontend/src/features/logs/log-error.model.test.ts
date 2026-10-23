import { expect, it } from 'vitest';
import chinese from '../../../../../internal/apierror/locales/zh-CN.json';
import { errorDiagnosis, errorExplanation } from './log-error.model';
import { safeSummary } from './log.model';
import { requestLog } from './log.service';
import fixture from '../../../../contract/logs.json';

it('uses core translations for transport, binding and mapping errors', () => {
  for (const code of ['binding_model_denied', 'upstream_transport', 'upstream_header_timeout', 'model_no_enabled_provider', 'unknown_model'] as const) {
    expect(errorExplanation(code)).toBe(chinese[code]);
  }
  expect(errorExplanation('upstream_header_timeout', 'en')).toContain('response headers');
  expect(errorExplanation('unknown-new-error')).toContain('无法还原');
  expect(errorExplanation('constructor')).toContain('无法还原');
  expect(errorExplanation('candidate_conversion_unsupported')).not.toContain('%s');
});
it('explains actual network cause, phase and timeout without inventing old evidence', () => {
  const evidence = { transport_cause: 'timeout', transport_phase: 'response_headers', provider_timeout_seconds: '500' };
  const diagnosis = errorDiagnosis('upstream_header_timeout', evidence);
  expect(diagnosis).toContain('HTTP 响应头');
  expect(diagnosis).toContain('500 秒');
  expect(errorDiagnosis('upstream_transport', {})).not.toContain('30');
  expect(errorDiagnosis('upstream_transport', { transport_cause: 'private-error-text' })).not.toContain('private-error-text');
  const row = requestLog({ ...fixture.page.requests[0], code: 'upstream_header_timeout', evidence });
  expect(safeSummary(row)).toContain(diagnosis);
});

it('uses the exact Anchor identity and fixed upstream category without remote text', () => {
  expect(errorDiagnosis('invalid_upstream_response', { anchor_error_id: 'upstream_incomplete' })).toBe(chinese.upstream_incomplete);
  expect(errorDiagnosis('upstream_rejected', { upstream_error_category: 'insufficient_quota' })).toContain('余额或配额不足');
  expect(errorDiagnosis('upstream_rejected', { upstream_error_category: 'private-upstream-text' })).not.toContain('private-upstream-text');
  expect(errorExplanation('invalid_upstream_response')).not.toContain('JSON');
});

it('explains configured first output timeout separately from header timeout', () => {
  expect(errorDiagnosis('upstream_first_token_timeout', { timeout_kind: 'first_token', first_token_timeout_seconds: '30', provider_timeout_seconds: '500' })).toContain('首字超时配置为 30 秒');
});

it('explains rejected reasoning effort and includes the actual effort in copied diagnostics', () => {
  const evidence = { upstream_error_category: 'unsupported_reasoning_effort' };
  const row = requestLog({ ...fixture.page.requests[0], code: 'upstream_rejected', reasoning_observed: true, reasoning_effort: 'none', evidence });
  expect(safeSummary(row)).toContain('上游不支持请求中的推理强度');
  expect(safeSummary(row)).toContain('请求思考强度: none');
  expect(safeSummary({ ...row, reasoningEffort: null })).toContain('请求思考强度: 未指定');
  expect(safeSummary({ ...row, reasoningEffort: undefined })).toContain('请求思考强度: 未记录');
});
