// Narrow adapter for the window-only API injected by Wails v2.15.0.
// Business views must not access window.runtime directly.
interface WindowRuntime {
  WindowMinimise(): void;
  WindowToggleMaximise(): void;
  WindowIsMaximised(): Promise<boolean>;
  Quit(): void;
}

const methods = ['WindowMinimise', 'WindowToggleMaximise', 'WindowIsMaximised', 'Quit'] as const;
function runtime(): WindowRuntime | null {
  const value = (window as Window & { runtime?: Partial<WindowRuntime> }).runtime;
  if (!value || !methods.every(method => typeof value[method] === 'function')) return null;
  return value as WindowRuntime;
}
function requireRuntime(): WindowRuntime {
  const value = runtime();
  if (!value) throw new Error('window_runtime_unavailable');
  return value;
}

export const windowRuntime = {
  available: () => runtime() !== null,
  minimise: () => requireRuntime().WindowMinimise(),
  toggleMaximise: () => requireRuntime().WindowToggleMaximise(),
  isMaximised: () => requireRuntime().WindowIsMaximised(),
  // Quit the GUI host only. Never call the core's shutdown endpoint here.
  close: () => requireRuntime().Quit(),
};
