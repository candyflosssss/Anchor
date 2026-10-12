import { useEffect, useRef, useState } from 'react';
import { agentMessage, agentService, matchingTemplate, type AgentTarget, type AgentWorkspace, type ManagedAgent } from './agent.model';

export function useAgentController(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [data, setData] = useState<AgentWorkspace | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [targetId, setTargetId] = useState('');
  const [model, setModel] = useState('');
  const [templates, setTemplates] = useState<Record<string, string>>({});
  const [approved, setApproved] = useState(false);
  const [disableTarget, setDisableTarget] = useState<ManagedAgent | null>(null);
  const alive = useRef(false);
  const epoch = useRef(0);
  const latch = useRef(false);
  useEffect(() => {
    alive.current = true;
    const generation = ++epoch.current;
    if (enabled) void agentService.workspace().then(v => { if (alive.current && epoch.current === generation) setData(v); })
      .catch(e => { if (alive.current && epoch.current === generation) setError(agentMessage(e)); })
      .finally(() => { if (alive.current && epoch.current === generation) setLoading(false); });
    return () => { alive.current = false; };
  }, [enabled]);
  const busy = loading || writing;
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  async function read() {
    if (!enabled || latch.current) return;
    latch.current = true; setLoading(true); setError(''); setNotice('');
    const generation = ++epoch.current;
    try { const v = await agentService.workspace(); if (alive.current && epoch.current === generation) { setData(v); setTargetId(''); setApproved(false); } }
    catch (e) { if (alive.current && epoch.current === generation) setError(agentMessage(e)); }
    finally { latch.current = false; if (alive.current && epoch.current === generation) setLoading(false); }
  }
  function select(target: AgentTarget) {
    if (busy || !data) return;
    const binding = data.bindings.find(b => sameRoot(b.config_root, target.config_root));
    setTargetId(target.id); setModel(binding?.model ?? data.models.find(m => m.tools && m.streaming && matchingTemplate(target, m))?.id ?? '');
    setTemplates(Object.fromEntries(data.models.map(m => [m.id, binding?.model_templates[m.id] ?? matchingTemplate(target, m)])));
    setApproved(false); setError(''); setNotice('');
  }
  async function write(action: () => Promise<AgentWorkspace>) {
    if (!enabled || busy || latch.current) return;
    latch.current = true; setWriting(true); setError(''); setNotice('');
    const generation = ++epoch.current;
    try {
      const v = await action();
      if (alive.current && epoch.current === generation) { setData(v); setTargetId(''); setApproved(false); setNotice('托管状态已更新。配置应用或恢复后请完全退出并重新启动 Codex；配置成功仍需在 Agent 中验证实际请求。'); }
    } catch (e) {
      if (alive.current && epoch.current === generation) {
        setError(agentMessage(e)); setApproved(false); setTargetId('');
        // Read after partial/conflicting writes. Never automatically replay them.
        try { const v = await agentService.workspace(); if (alive.current && epoch.current === generation) setData(v); }
        catch { if (alive.current && epoch.current === generation) setData(null); }
      }
    } finally { latch.current = false; if (alive.current && epoch.current === generation) { setWriting(false); setDisableTarget(null); } }
  }
  const selected = data?.targets.find(t => t.id === targetId);
  const binding = selected ? data?.bindings.find(b => sameRoot(b.config_root, selected.config_root)) : undefined;
  return { data, loading, busy, error, notice, selected, binding, model, setModel, templates, approved, setApproved, disableTarget, setDisableTarget,
    read, select, cancel: () => { if (!busy) setTargetId(''); },
    chooseTemplate: (alias: string, source: string) => { setTemplates(v => ({ ...v, [alias]: source })); setApproved(false); },
    enroll: () => selected && approved && write(() => agentService.enroll({ target_id: selected.id, model, expected_revision: binding?.revision ?? 0,
      model_templates: templates, approved: true })),
    manage: (row: ManagedAgent, action: 'sync' | 'enable' | 'disable') => write(() => agentService.manage({ binding_id: row.id, expected_revision: row.revision, action })),
  };
}
// Windows targets may retain an earlier spelling in a persisted binding.
function sameRoot(a: string, b: string) { return a === b || (/^[A-Za-z]:[\\/]/.test(a) && a.replaceAll('/', '\\').toLowerCase() === b.replaceAll('/', '\\').toLowerCase()); }
