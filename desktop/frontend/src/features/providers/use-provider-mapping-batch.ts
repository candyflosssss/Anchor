import { useEffect, useState } from 'react';
import type { Workspace } from './provider-features.model';
import type { MappingBatchPatch, MappingBatchReport } from './provider-mapping-batch.model';
import type { ProviderDialog } from './provider-summary.model';
import type { ProviderController } from './use-provider-controller';

export function useMappingSelection(workspace: Workspace) {
  const [selection, setSelection] = useState<{ provider: string; ids: string[] }>({ provider: workspace.provider_id, ids: [] });
  useEffect(() => {
    setSelection(previous => {
      const ids = previous.provider === workspace.provider_id ? previous.ids.filter(id => workspace.models.some(model => model.id === id)) : [];
      return previous.provider === workspace.provider_id && ids.length === previous.ids.length ? previous : { provider: workspace.provider_id, ids };
    });
  }, [workspace.provider_id, workspace.models]);
  const ids = selection.provider === workspace.provider_id ? selection.ids.filter(id => workspace.models.some(model => model.id === id)) : [];
  const set = (next: string[]) => setSelection({ provider: workspace.provider_id, ids: next });
  return {
    ids, all: !!workspace.models.length && ids.length === workspace.models.length,
    toggle: (id: string, checked: boolean) => set(checked ? [...new Set([...ids, id])] : ids.filter(value => value !== id)),
    selectAll: (checked: boolean) => set(checked ? workspace.models.map(model => model.id) : []),
    clear: () => set([]),
  };
}

export function useMappingBatchDialog(c: ProviderController, d: Extract<ProviderDialog, { kind: 'mapping-batch' }>) {
  const [patch, setPatch] = useState<MappingBatchPatch>({});
  const [report, setReport] = useState<MappingBatchReport | null>(null);
  const [stopping, setStopping] = useState(false);
  const busy = c.features.busy;
  const canStart = !busy && !report && !c.features.needsRead && d.ids.length > 0 && (d.operation !== 'edit' || Object.keys(patch).length > 0);
  function change<K extends keyof MappingBatchPatch>(key: K, value: MappingBatchPatch[K]) {
    setPatch(previous => {
      const next = { ...previous };
      if (value === undefined) delete next[key]; else next[key] = value;
      return next;
    });
  }
  return {
    patch, change, report, busy, stopping, canStart,
    start: () => { if (canStart) void c.features.runMappingBatch(d.workspace, d.ids, d.operation, patch, setReport); },
    stop: () => { setStopping(true); c.features.stopMappingBatch(); },
    close: () => { if (!busy) c.closeDialog(); },
  };
}
