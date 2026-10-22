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
