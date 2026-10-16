import type { ChatMessage, ChatRequest, ProbeProgress, ProviderModels, Workspace } from '../../desktop/contract.generated';
import { bridge, call, DesktopProblem } from '../../desktop/transport';
import { ProviderProblem } from '../providers/provider.model';
import { checkedProgress } from '../providers/provider-test.model';

export const MAX_CHAT_BYTES = 64 * 1024;
export const MAX_CHAT_MESSAGES = 80;
export type { ChatMessage, ChatRequest, ProbeProgress };
export interface ChatTurn { content: string; reply: string; status: 'running' | 'completed' | 'cancelled' | 'failed' }
export interface ChatTransport { request(input: ChatRequest, signal: AbortSignal): Promise<ProbeProgress> }
export class DesktopChatTransport implements ChatTransport {
  constructor(private readonly getBridge = bridge) {}
  async request(input: ChatRequest, signal: AbortSignal) {
    signal.throwIfAborted();
    if (typeof this.getBridge().ProviderChat !== 'function') throw new DesktopProblem('unsupported');
    const result = await call(() => this.getBridge().ProviderChat(input), input.action === 'start');
    signal.throwIfAborted();
    const progress = checkedProgress(result, input, MAX_CHAT_BYTES);
    if (progress.key_id !== input.key_id || progress.model !== input.model) throw new DesktopProblem('read_error');
    return progress;
  }
}
export function chatModels(workspace: Workspace, key: string, catalog: ProviderModels | null): string[] {
  const saved = workspace.models.filter(m => !m.key_ids.length || m.key_ids.includes(key)).map(m => m.upstream);
  const discovered = catalog?.entries.filter(m => m.key_ids.includes(key)).map(m => m.id) ?? [];
  return [...new Set([...saved, ...discovered])].sort((a, b) => a.localeCompare(b));
}
export function chatMessages(turns: ChatTurn[], content: string): ChatMessage[] {
  // Failed or cancelled turns are visible, but never silently submitted as context.
  return [...turns.filter(t => t.status === 'completed').flatMap(t => [
    { role: 'user', content: t.content }, { role: 'assistant', content: t.reply },
  ]), { role: 'user', content }];
}
export function messageProblem(messages: ChatMessage[]): string {
  if (messages.length > MAX_CHAT_MESSAGES) return '对话已达到 80 条消息上限，请清空对话后继续。';
  if (messages.some(m => !m.content.trim())) return '消息不能为空。';
  if (messages.reduce((n, m) => n + new TextEncoder().encode(m.content).length, 0) > MAX_CHAT_BYTES) return '对话文本超过 64 KiB，请清空对话或缩短消息。';
  return '';
}
export function chatProblem(error: unknown): string {
  const code = error instanceof DesktopProblem || error instanceof ProviderProblem ? error.code : '';
  if (code === 'conflict') return '供应商配置已变化，请刷新配置后重试。不会自动重发消息。';
  if (code === 'unsupported') return '当前桌面宿主或核心不支持对话测试，请更新后重试。';
  if (code === 'unauthorized') return '当前连接没有管理授权，请重新连接。';
  if (code === 'invalid') return '供应商、Key 或消息不可用，请刷新配置并检查输入。';
  if (code === 'unknown') return '发送结果未知，请先核对日志。为避免重复计费，没有自动重试。';
  return '对话请求中断或设备离线。没有自动重试，请检查连接与日志。';
}
export function chatFailure(code: string): string {
  const text: Record<string, string> = {
    busy: '当前并发已满，请稍后手动重试。', authentication: '上游拒绝了此 Key，请检查授权。',
    rate_limited: '上游限流，请稍后手动重试。', network: '上游网络连接失败。', timeout: '生成超时（最多 120 秒，受供应商超时限制）。',
    invalid_response: '上游响应无效、不完整或超过安全大小限制。', unexpected_html: '上游返回了网页，请检查 API 地址。', http_error: '上游返回了 HTTP 错误，请查看日志。',
  };
  return text[code] ?? '未能生成完整回复，请检查日志。';
}
