import fixture from '../../../../contract/chat.json';
import type { ChatRequest, ChatTransport, ProbeProgress } from './chat.model';

/** Explicit prototype only; never used as a fallback by a desktop connection. */
export class MockChatTransport implements ChatTransport {
  private runs = new Map<string, { input: ChatRequest; step: number; cancelled: boolean }>();
  async request(input: ChatRequest, signal: AbortSignal): Promise<ProbeProgress> {
    signal.throwIfAborted();
    if (input.action === 'start') {
      this.runs.clear();
      const id = crypto.randomUUID();
      this.runs.set(id, { input, step: 0, cancelled: false });
      return this.progress(id);
    }
    const run = this.runs.get(input.run_id);
    if (!run) throw new Error('missing mock run');
    if (input.action === 'cancel') run.cancelled = true;
    else run.step++;
    return this.progress(input.run_id);
  }
  private progress(id: string): ProbeProgress {
    const run = this.runs.get(id)!;
    const { input, step, cancelled } = run;
    const done = cancelled || step >= 8;
    const text = `这是 Mock 对话预览，不会请求真实供应商。\n\n${fixture.progress.text}`;
    const out: ProbeProgress = { ...fixture.progress, provider_id: input.provider_id, revision: input.expected_revision,
      key_id: input.key_id, model: input.model, run_id: id, elapsed_ms: step * 150,
      first_byte_ms: step ? Math.min(112, step * 150) : 0,
      text: text.slice(0, step * 10), status: cancelled ? 'cancelled' : done ? 'completed' : 'running', phase: done ? 'complete' : 'receiving',
      result: done ? { ...fixture.progress.result, revision: input.expected_revision, key_id: input.key_id, model: input.model } : null };
    return out;
  }
}
