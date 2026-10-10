import { Copy, Minus, Square, X } from 'lucide-react';
import { windowMessages } from './window.model';
import type { WindowController } from './use-window-controller';

export function WindowControls({ controller }: { controller: WindowController }) {
  const maximiseLabel = controller.maximised === null ? windowMessages.toggle : controller.maximised ? windowMessages.restore : windowMessages.maximise;
  const MaximiseIcon = controller.maximised ? Copy : Square;
  return <div className="window-controls" data-window-no-drag role="group" aria-label="窗口控制">
    <button type="button" disabled={!controller.available} aria-label={windowMessages.minimise} title={windowMessages.minimise} onClick={controller.minimise}><Minus aria-hidden="true" /></button>
    <button type="button" disabled={!controller.available} aria-label={maximiseLabel} title={maximiseLabel} onClick={controller.toggleMaximise}><MaximiseIcon aria-hidden="true" /></button>
    <button type="button" className="window-close" disabled={!controller.available} aria-label={windowMessages.close} title={windowMessages.closeHint} onClick={controller.close}><X aria-hidden="true" /></button>
  </div>;
}
