import type { RequestLog, SystemLog } from './log.model';
const attempt = (provider: string, key: string, model: string, http: number | null, elapsed: number, outcome = '成功', note = '响应校验通过，已完成交付。') => ({ number: 1, provider, key, model, protocol: 'Responses', http, elapsed, outcome, note });
const baseFixtures: RequestLog[] = [
  { id: 'req_demo_8f2c91', ageMinutes: 1, time: '23:10:18', status: 'failed', source: 'probe', model: 'codex-auto-review', protocol: 'Responses', elapsed: 2606, http: 200, provider: '摸鱼AI', key: 'default', revision: 2, delivery: '测试结果已返回', summary: '上游返回成功状态，但响应缺少协议必需字段。', stage: 'response_validate', code: 'invalid_response', input: null, output: null,
    attempts: [{ ...attempt('摸鱼AI', 'default', 'codex-auto-review', 200, 2606, '校验失败', '缺少响应 ID，无法确认响应符合 Responses 协议。'), code: 'missing_required_field' }], evidence: { expected_format: 'Responses JSON', json_parse: 'passed', has_response_id: false, response_bytes: 824, max_output_tokens: 16 } },
  { id: 'req_demo_b7a403', ageMinutes: 2, time: '23:09:42', status: 'succeeded', source: 'proxy', model: 'coding-fast', protocol: 'Responses', elapsed: 1834, http: 200, provider: 'OpenAI', key: 'key-2', revision: 5, delivery: '完整写出', summary: '首次请求触发限流，切换候选后成功。', stage: 'respond', input: 1840, output: 326,
    attempts: [{ ...attempt('摸鱼AI', 'default', 'gpt-5.4', 429, 216, '上游限流', '收到 429，在输出前切换到下一候选。'), code: 'rate_limited' }, { ...attempt('OpenAI', 'key-2', 'gpt-5.4', 200, 1618), number: 2 }], evidence: { retry_reason: 'rate_limited', downstream_started_before_retry: false, usage_scope: '已知小计；首次尝试用量未知' } },
  { id: 'req_demo_c102ee', ageMinutes: 4, time: '23:08:11', status: 'succeeded', source: 'proxy', model: 'claude-sonnet', protocol: 'Responses → Anthropic', elapsed: 972, http: 200, provider: 'Anthropic', key: 'primary', revision: 3, delivery: '完整写出', summary: '协议转换完成，响应已写出。', stage: 'respond', input: 2184, output: 482,
    attempts: [{ ...attempt('Anthropic', 'primary', 'claude-sonnet-4-6', 200, 972), protocol: 'Anthropic' }], evidence: { conversion: 'Responses → Anthropic → Responses', first_byte_ms: 284, generation_status: 'completed' } },
  { id: 'req_demo_d39e20', ageMinutes: 6, time: '23:06:24', status: 'failed', source: 'proxy', model: 'coding-fast', protocol: 'Responses · 流式', elapsed: 8120, http: 200, provider: '摸鱼AI', key: 'default', revision: 2, delivery: '已输出部分内容', summary: '上游流在终止事件到达前断开，已有部分内容写出。', stage: 'respond', code: 'stream_interrupted', input: null, output: null,
    attempts: [attempt('摸鱼AI', 'default', 'gpt-5.4', 200, 8120, '流式中断', '已开始输出，未自动切换供应商或重放。')], evidence: { response_bytes: 4096, terminal_event_present: false, delivery: 'partial', replayed: false } },
  { id: 'req_demo_e910af', ageMinutes: 9, time: '23:03:05', status: 'succeeded', source: 'probe', model: 'codex-auto-review', protocol: 'Responses', elapsed: 1482, http: 200, provider: '摸鱼AI', key: 'default', revision: 2, delivery: '测试结果已返回', summary: '协议校验通过；生成达到本次测试的输出上限。', stage: 'response_validate', input: 24, output: 16,
    attempts: [attempt('摸鱼AI', 'default', 'codex-auto-review', 200, 1482, '协议通过', '合法 reasoning 输出；达到测试上限不等于响应结构无效。')], evidence: { recognized_output_types: 'reasoning', generation_status: 'incomplete', incomplete_reason: 'max_output_tokens', max_output_tokens: 16 } },
  { id: 'req_demo_f21d64', ageMinutes: 12, time: '23:00:17', status: 'cancelled', source: 'proxy', model: 'claude-sonnet', protocol: 'Anthropic · 流式', elapsed: 2380, http: 200, provider: 'Anthropic', key: 'primary', revision: 3, delivery: '已输出部分内容', summary: '客户端主动取消，核心停止继续读取上游。', stage: 'cleanup', code: 'client_cancelled', input: null, output: null,
    attempts: [attempt('Anthropic', 'primary', 'claude-sonnet-4-6', 200, 2380, '已取消', '取消来自客户端，不计为供应商认证失败。')], evidence: { cancellation_source: 'client', delivery: 'partial' } },
  { id: 'req_demo_a098bc', ageMinutes: 17, time: '22:55:33', status: 'failed', source: 'proxy', model: 'local-review', protocol: 'Responses', elapsed: 3, http: null, provider: '—', key: '—', revision: 1, delivery: '未输出生成内容', summary: '没有可用的模型候选，请求未发送到上游。', stage: 'route', code: 'no_available_candidate', input: null, output: null, attempts: [], evidence: { upstream_dispatched: false } },
  { id: 'req_demo_109ea4', ageMinutes: 35, time: '22:37:08', status: 'interrupted', source: 'proxy', model: 'coding-fast', protocol: 'Responses', elapsed: null, http: null, provider: 'OpenAI', key: 'key-2', revision: 5, delivery: '最终交付未知', summary: '上次核心异常退出，这次请求的最终结果未知。', stage: 'cleanup', code: 'core_interrupted', input: null, output: null,
    attempts: [attempt('OpenAI', 'key-2', 'gpt-5.4', null, 0, '结果未知', '没有完成记录，不能判定上游是否已执行或计费。')], evidence: { previous_run: 'run_demo_previous', recovered_as: 'interrupted' } },
  { id: 'req_demo_34abc0', ageMinutes: 95, time: '21:37:24', status: 'succeeded', source: 'proxy', model: 'coding-fast', protocol: 'Responses', elapsed: 646, http: 200, provider: 'OpenAI', key: 'key-2', revision: 5, delivery: '完整写出', summary: '响应校验通过，已完成交付。', stage: 'respond', input: 860, output: 140,
    attempts: [attempt('OpenAI', 'key-2', 'gpt-5.4', 200, 646)], evidence: { generation_status: 'completed' } },
];
// Explicit synthetic request snapshots; absent timing/usage stays unknown.
export const requestFixtures: RequestLog[] = baseFixtures.map(row => {
  const snapshots: Record<string, Partial<RequestLog>> = {
    req_demo_8f2c91: { reasoningEffort: 'high', stream: false },
    req_demo_b7a403: { reasoningEffort: 'medium', stream: true, generationStatus: 'completed', metrics: { timeToFirstTextMs: 584, outputGenerationMs: 1200, nonReasoningOutputTokens: 326, outputIntervalComplete: true, generationMs: 1200, outputTextTokens: 326, textIntervalComplete: true, cacheReadTokens: 1024, reasoningTokens: 0 } },
    req_demo_c102ee: { reasoningEffort: null, stream: true, generationStatus: 'completed', metrics: { timeToFirstTextMs: 284, outputGenerationMs: 640, nonReasoningOutputTokens: 482, outputIntervalComplete: true, generationMs: 640, outputTextTokens: 482, textIntervalComplete: true, cacheReadTokens: 1536, reasoningTokens: 0 } },
    req_demo_d39e20: { reasoningEffort: 'high', stream: true, metrics: { timeToFirstTextMs: 820, outputGenerationMs: null, nonReasoningOutputTokens: null, outputIntervalComplete: false, generationMs: null, outputTextTokens: null, textIntervalComplete: false, cacheReadTokens: null, reasoningTokens: null } },
    req_demo_e910af: { reasoningEffort: 'high', stream: false, generationStatus: 'incomplete', metrics: { timeToFirstTextMs: null, outputGenerationMs: null, nonReasoningOutputTokens: 0, outputIntervalComplete: false, generationMs: null, outputTextTokens: 0, textIntervalComplete: false, cacheReadTokens: null, reasoningTokens: 16 } },
    req_demo_f21d64: { reasoningEffort: null, stream: true, metrics: { timeToFirstTextMs: 390, outputGenerationMs: null, nonReasoningOutputTokens: null, outputIntervalComplete: false, generationMs: null, outputTextTokens: null, textIntervalComplete: false, cacheReadTokens: null, reasoningTokens: null } },
    req_demo_a098bc: { reasoningEffort: 'low', stream: false },
    req_demo_109ea4: { reasoningEffort: 'medium', stream: true },
    req_demo_34abc0: { reasoningEffort: 'low', stream: true, generationStatus: 'completed', metrics: { timeToFirstTextMs: 180, outputGenerationMs: 420, nonReasoningOutputTokens: 140, outputIntervalComplete: true, generationMs: 420, outputTextTokens: 140, textIntervalComplete: true, cacheReadTokens: 0, reasoningTokens: 0 } },
  };
  return { ...row, ...snapshots[row.id] };
});
export const incomingFixture: RequestLog = { ...requestFixtures[8], id: 'req_demo_new001', ageMinutes: 0, time: '23:12:00', elapsed: 721, attempts: [{ ...requestFixtures[8].attempts[0], elapsed: 721 }] };
export const systemFixtures: SystemLog[] = [
  { id: 'evt_demo_01', ageMinutes: 1, time: '23:10:18', level: 'warn', source: 'core', event: 'probe.validation_failed', message: '推理测试响应校验未通过，上游 HTTP 200。', requestId: 'req_demo_8f2c91' },
  { id: 'evt_demo_02', ageMinutes: 2, time: '23:09:42', level: 'info', source: 'core', event: 'request.retry_succeeded', message: '第二次尝试成功，未在重试前输出内容。', requestId: 'req_demo_b7a403' },
  { id: 'evt_demo_03', ageMinutes: 6, time: '23:06:24', level: 'error', source: 'core', event: 'stream.interrupted', message: '流式响应中断；已输出部分内容，未重放。', requestId: 'req_demo_d39e20' },
  { id: 'evt_demo_04', ageMinutes: 20, time: '22:52:00', level: 'info', source: 'desktop', event: 'desktop.core_connected', message: '已核验实例身份，复用本机核心。' },
  { id: 'evt_demo_05', ageMinutes: 20, time: '22:51:59', level: 'warn', source: 'core', event: 'logging.pending_recovered', message: '恢复 1 条未结束记录，标记为核心中断。', requestId: 'req_demo_109ea4' },
  { id: 'evt_demo_06', ageMinutes: 21, time: '22:51:58', level: 'info', source: 'core', event: 'core.started', message: '核心启动，结构化日志写入已就绪。' },
];
