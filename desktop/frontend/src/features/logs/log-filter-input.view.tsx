import { useEffect, useId, useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { Input } from '../../components/ui/input';
import type { LogTimeRange } from './log.model';
import { useLogSuggestions, type SuggestionField } from './use-log-suggestions';

type Props = {
  label: string; placeholder: string; field: SuggestionField; value: string;
  kind: 'requests' | 'system'; window: string; revision: number;
  range?: LogTimeRange | null;
  onChange: (value: string) => void;
};

export function LogFilterInput({ label, placeholder, field, value, kind, window, revision, range, onChange }: Props) {
  const id = useId(), list = useRef<HTMLUListElement>(null);
  const [open, setOpen] = useState(false), [active, setActive] = useState(-1);
  const [composing, setComposing] = useState(false), [draft, setDraft] = useState('');
  const suggestions = useLogSuggestions(kind, window, field, value, open && !composing, revision, range);
  function choose(next: string) { onChange(next); setOpen(false); setActive(-1); }
  useEffect(() => { list.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }); }, [active]);
  return <div className="log-filter-input" onBlur={() => { setOpen(false); setActive(-1); }}>
    <Input role="combobox" aria-label={label} aria-autocomplete="list" aria-expanded={open && !composing}
      aria-controls={open && !composing ? id : undefined} aria-activedescendant={open && active >= 0 && suggestions.items[active] ? `${id}-${active}` : undefined}
      autoComplete="off" placeholder={placeholder} value={composing ? draft : value}
      onFocus={() => { setOpen(true); setActive(-1); }} onClick={() => setOpen(true)}
      onCompositionStart={() => { setDraft(value); setComposing(true); setActive(-1); }}
      onCompositionEnd={e => { setComposing(false); onChange(e.currentTarget.value); }}
      onChange={e => { if (composing) setDraft(e.target.value); else onChange(e.target.value); setActive(-1); setOpen(true); }}
      onKeyDown={e => {
        if (composing || e.nativeEvent.isComposing || e.keyCode === 229) return;
        if (e.key === 'Escape') { if (open) { e.preventDefault(); e.stopPropagation(); } setOpen(false); setActive(-1); }
        else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
          e.preventDefault(); setOpen(true);
          const n = suggestions.items.length;
          setActive(n ? e.key === 'ArrowDown' ? (active + 1) % n : (active <= 0 ? n - 1 : active - 1) : -1);
        } else if (e.key === 'Enter' && open && active >= 0 && suggestions.items[active]) {
          e.preventDefault(); choose(suggestions.items[active].value);
        }
      }} />
    <ChevronDown size={13} aria-hidden="true" className="log-filter-chevron" />
    {open && !composing && <div className="log-filter-popover">
      <ul id={id} role="listbox" aria-label={`${label}候选`} ref={list}>
        {suggestions.items.map((item, index) => <li id={`${id}-${index}`} key={item.value} role="option" aria-selected={index === active}
          onPointerDown={e => e.preventDefault()} onPointerMove={() => setActive(index)} onClick={() => choose(item.value)}>
          <span className="log-filter-value">{item.value}</span><span className="log-filter-count" aria-label={`${item.count} 条日志`}>{item.count.toLocaleString()}</span>
        </li>)}
      </ul>
      {suggestions.loading ? <div className="log-filter-state" role="status">读取候选…</div>
        : suggestions.failed ? <div className="log-filter-state" role="status">候选不可用，可手动输入</div>
          : suggestions.items.length === 0 ? <div className="log-filter-state" role="status">暂无匹配候选</div>
            : suggestions.hasMore ? <div className="log-filter-state">输入前缀查看更多</div> : null}
    </div>}
  </div>;
}
