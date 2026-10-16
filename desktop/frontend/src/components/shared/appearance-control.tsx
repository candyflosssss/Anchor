import { Check, Monitor, Moon, Sun } from 'lucide-react';
import type { Appearance } from '../../app/use-appearance';

const appearances = [
  ['light', Sun, '浅色'], ['dark', Moon, '深色'], ['system', Monitor, '跟随系统'],
] as const;

/** The same preference controls both the sidebar shortcut and the settings page. */
export function AppearanceControl({ value, onChange, labelled = false }: {
  value: Appearance; onChange: (value: Appearance) => void; labelled?: boolean;
}) {
  return <div className={labelled ? 'appearance-options' : 'appearance-control'} role="group" aria-label="外观设置">
    {appearances.map(([appearance, Icon, label]) => <button key={appearance} type="button" aria-label={label} title={labelled ? undefined : label} aria-pressed={value === appearance} onClick={() => onChange(appearance)}>
      <Icon size={16} aria-hidden="true" />
      {labelled && <><span>{label}</span><Check size={16} className="appearance-check" aria-hidden="true" /></>}
    </button>)}
  </div>;
}
