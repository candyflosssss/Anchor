import { useEffect, useRef, useState } from 'react';
import { agentMessage, agentService, availableAgentModels, agentRows, type AgentRow, type AgentWorkspace, type ManagedAgent, type EnrollAgent } from './agent.model';

export function useAgentController(enabled: boolean, onActivity: (busy: boolean) => void) {
  const [data, setData] = useState<AgentWorkspace | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [targetId, setTargetId] = useState('');
  const [model, setModel] = useState('');
  const [disableTarget, setDisableTarget] = useState<ManagedAgent | null>(null);
  const [updateInput, setUpdateInput] = useState<EnrollAgent | null>(null);
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
    try { const v = await agentService.workspace(); if (alive.current && epoch.current === generation) { setData(v); setTargetId(''); } }
    catch (e) { if (alive.current && epoch.current === generation) setError(agentMessage(e)); }
    finally { latch.current = false; if (alive.current && epoch.current === generation) setLoading(false); }
  }
  const rows = data ? agentRows(data) : [];
  function select(row: AgentRow) {
    if (busy || !data) return;
    const available = row.target ? availableAgentModels(data, row.target) : [];
    setTargetId(row.id); setModel(available.find(m => m.id === row.binding?.model)?.id ?? available[0]?.id ?? '');
    setError(''); setNotice('');
  }
  async function write(action: () => Promise<AgentWorkspace>, affectedRow = targetId) {
    if (!enabled || busy || latch.current) return;
    latch.current = true; setWriting(true); setError(''); setNotice('');
    const generation = ++epoch.current;
    try {
      const v = await action();
      if (alive.current && epoch.current === generation) { setData(v); setTargetId(''); setNotice('托管状态已更新。配置应用或恢复后请完全退出并重新启动对应 Agent（DSH Desktop 关闭窗口可能只是隐藏，请完全退出；DSH 请新建会话）；配置成功仍需在 Agent 中验证实际请求。'); }
    } catch (e) {
      if (alive.current && epoch.current === generation) {
        setError(agentMessage(e)); setTargetId(affectedRow);
        // Read after partial/conflicting writes. Never automatically replay them.
        try {
          const v = await agentService.workspace();
          if (alive.current && epoch.current === generation) {
            setData(v);
            const row = agentRows(v).find(r => r.id === affectedRow);
            if (row?.target) {
              const choices = availableAgentModels(v, row.target);
              setModel(current => choices.some(m => m.id === current) ? current : choices.find(m => m.id === row.binding?.model)?.id ?? choices[0]?.id ?? '');
            }
            setError(`${agentMessage(e)} 已重新读取当前状态。`);
          }
        } catch {
          if (alive.current && epoch.current === generation) {
            setData(null);
            setError(`${agentMessage(e)} 重新读取也失败了，当前结果尚未确认，请重新识别；不要重复提交。`);
          }
        }
      }
    } finally { latch.current = false; if (alive.current && epoch.current === generation) { setWriting(false); setDisableTarget(null); setUpdateInput(null); } }
  }
  const expanded = rows.find(row => row.id === targetId);
  const selected = expanded?.target;
  const binding = expanded?.binding;
  const availableModels = selected && data ? availableAgentModels(data, selected) : [];
  return { data, rows, expanded, availableModels, loading, busy, error, notice, selected, binding, model, setModel, disableTarget, setDisableTarget, updateInput, setUpdateInput,
    read, select, cancel: () => { if (!busy) setTargetId(''); },
    update: (rebuild = false) => {
      if (busy || !selected?.supported || (rebuild ? selected.agent !== 'codex' || !['ready', 'config_unsupported'].includes(selected.status) : selected.status !== 'ready') || !availableModels.some(m => m.id === model)) return;
      const input: EnrollAgent = { target_id: selected.id, model, expected_revision: binding?.revision ?? 0,
        model_scope: 'all-mapped', model_templates: {}, approved: true, ...(rebuild ? { rebuild: true } : {}) };
      if (binding || rebuild || selected.agent === 'claude-desktop') setUpdateInput(input);
      else void write(() => agentService.enroll(input));
    },
    confirmUpdate: () => updateInput && write(() => agentService.enroll(updateInput)),
    manage: (row: ManagedAgent, action: 'enable' | 'disable') => write(() => agentService.manage({ binding_id: row.id, expected_revision: row.revision, action }), rows.find(v => v.binding?.id === row.id)?.id),
  };
}
