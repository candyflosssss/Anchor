import { useState } from 'react';
import { columnPresets, columnStorageKey, legacyColumnStorageKeys, defaultColumns, logColumns, moveColumn, normalizeColumns, type LogColumnId } from './log-columns.model';

function readColumns(): LogColumnId[] {
  try {
    const saved = [columnStorageKey, ...legacyColumnStorageKeys].map(key => localStorage.getItem(key)).find(value => value !== null);
    return normalizeColumns(JSON.parse(saved ?? 'null'));
  }
  catch { return [...defaultColumns]; }
}
export function useLogColumnsController() {
  const [ids, setIds] = useState(readColumns);
  const [open, setOpen] = useState(false);
  const [storageError, setStorageError] = useState(false);
  function update(next: LogColumnId[]) {
    setIds(next);
    try { localStorage.setItem(columnStorageKey, JSON.stringify(next)); setStorageError(false); }
    catch { setStorageError(true); }
  }
  function toggle(id: LogColumnId) {
    if (ids.includes(id)) { if (ids.length > 1) update(ids.filter(current => current !== id)); }
    else update([...ids, id]);
  }
  return {
    ids, open, setOpen, storageError, toggle,
    columns: ids.map(id => logColumns.find(column => column.id === id)!),
    move: (id: LogColumnId, direction: -1 | 1) => update(moveColumn(ids, id, direction)),
    preset: (key: keyof typeof columnPresets) => update([...columnPresets[key].ids]),
    reset: () => update([...defaultColumns]),
  };
}
export type LogColumnsController = ReturnType<typeof useLogColumnsController>;
