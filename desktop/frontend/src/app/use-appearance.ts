import { useEffect, useState } from 'react';
export type Appearance = 'system' | 'light' | 'dark';
const key = 'anchor.ui.appearance';
function readPreference(): Appearance {
  try { const value = localStorage.getItem(key); if (value === 'light' || value === 'dark') return value; } catch { /* Preference storage is optional. */ }
  return 'system';
}
export function useAppearance() {
  const [appearance, setAppearance] = useState<Appearance>(readPreference);
  useEffect(() => {
    const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : null;
    const apply = () => document.documentElement.classList.toggle('dark', appearance === 'dark' || (appearance === 'system' && !!media?.matches));
    apply(); media?.addEventListener('change', apply);
    try { localStorage.setItem(key, appearance); } catch { /* Keep the current session usable. */ }
    return () => media?.removeEventListener('change', apply);
  }, [appearance]);
  return { appearance, setAppearance };
}
