import mappingFixture from '../../../../contract/provider-model-mapping.json';
import { describe, expect, it } from 'vitest';
import fixture from '../../../../contract/chat.json';
import workspaceFixture from '../../../../contract/provider-workspace.json';
import catalogFixture from '../../../../contract/provider-models.json';
import { CONTRACT, type DesktopBridge, type ProbeProgress, type Workspace } from '../../desktop/contract.generated';
import { DesktopChatTransport, chatMessages, chatModels, messageProblem } from './chat.model';
import { MockChatTransport } from './chat.mock';

const signal = () => new AbortController().signal;
describe('text chat contract', () => {
  it('uses the shared fixture and preserves complete history', async () => {
    const api = { ProviderChat: async () => ({ contract: CONTRACT, data: fixture.progress, problem: null }) } as unknown as DesktopBridge;
    const result = await new DesktopChatTransport(() => api).request(fixture.request, signal());
    expect(result.text).toBe(fixture.progress.text);
    expect(result.key_id).toBe(fixture.request.key_id);
  });
  it('fails closed for a missing method, mismatch and unexpected data', async () => {
    await expect(new DesktopChatTransport(() => ({} as DesktopBridge)).request(fixture.request, signal())).rejects.toMatchObject({ code: 'unsupported' });
    for (const changes of [{ provider_id: 'other' }, { revision: 99 }, { key_id: 'wrong' }, { model: 'wrong' }, { text: 'x'.repeat(65537) }]) {
      const data = { ...fixture.progress, ...changes };
      const api = { ProviderChat: async () => ({ contract: CONTRACT, data, problem: null }) } as unknown as DesktopBridge;
      await expect(new DesktopChatTransport(() => api).request(fixture.request, signal())).rejects.toBeInstanceOf(Error);
    }
  });
  it('does not submit failed or stopped rounds as context', () => {
    const messages = chatMessages([
      { content: 'one', reply: 'two', status: 'completed' },
      { content: 'bad', reply: 'partial', status: 'cancelled' },
      { content: 'bad again', reply: '', status: 'failed' },
    ], 'three');
    expect(messages).toEqual([{ role: 'user', content: 'one' }, { role: 'assistant', content: 'two' }, { role: 'user', content: 'three' }]);
    expect(messageProblem(messages)).toBe('');
    expect(messageProblem([{ role: 'user', content: '中'.repeat(22000) }])).toContain('64 KiB');
    expect(messageProblem(Array.from({ length: 81 }, () => ({ role: 'user', content: 'x' })))).toContain('80');
  });
  it('keeps model suggestions bound to the selected key', () => {
    const workspace: Workspace = { ...workspaceFixture, models: [
      { ...mappingFixture, upstream: 'only-a', key_ids: ['a'] },
      { ...mappingFixture, upstream: 'shared', key_ids: [] },
    ] };
    const catalog = { ...catalogFixture.success, entries: [{ id: 'only-b', key_ids: ['b'] }, { id: 'shared', key_ids: ['a', 'b'] }] };
    expect(chatModels(workspace, 'a', catalog)).toEqual(['only-a', 'shared']);
    expect(chatModels(workspace, 'b', catalog)).toEqual(['only-b', 'shared']);
  });
  it('mock uses the same progress shape and supports cancellation', async () => {
    const transport = new MockChatTransport();
    const first = await transport.request(fixture.request, signal());
    expect(first.status).toBe('running');
    const read = { ...fixture.request, run_id: first.run_id, action: 'read', messages: [] };
    const partial = await transport.request(read, signal()); expect(partial.text).not.toBe('');
    const stopped: ProbeProgress = await transport.request({ ...read, action: 'cancel' }, signal());
    expect(stopped.status).toBe('cancelled');
    expect((await transport.request(read, signal())).status).toBe('cancelled');
  });
});
