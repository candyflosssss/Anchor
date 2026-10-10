import { useEffect } from 'react';
import { ProviderWorkspace } from '../features/providers/provider-workspace.view';
import { useProviderController } from '../features/providers/use-provider-controller';
import type { ProviderEnvironment } from '../desktop/environment';
export function ProvidersPage({ environment, onActivity }: { environment: ProviderEnvironment; onActivity: (busy: boolean) => void }) {
  const controller = useProviderController(environment);
  const busy = controller.busy || !!controller.editor || !!controller.toggle;
  useEffect(() => { onActivity(busy); return () => onActivity(false); }, [busy, onActivity]);
  return <ProviderWorkspace c={controller} />;
}
