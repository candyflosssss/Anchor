import { CircleCheck, CircleMinus } from 'lucide-react';
export function StatusBadge({ enabled }: { enabled: boolean }) {
  const Icon = enabled ? CircleCheck : CircleMinus;
  return <span className={`status-badge ${enabled ? 'status-enabled' : 'status-disabled'}`}><Icon size={13} />{enabled ? '已启用' : '已停用'}</span>;
}
