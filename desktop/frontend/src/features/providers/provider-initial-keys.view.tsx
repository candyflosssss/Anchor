import { Eye, EyeOff, Plus, X } from 'lucide-react';
import { useState } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { MAX_PROVIDER_KEYS, nextInitialKey, protocolName, type FieldErrors, type ProviderDraft } from './provider.model';

type Keys = ProviderDraft['initialKeys'];
export function ProviderInitialKeys({ draft, errors, onChange }: { draft: ProviderDraft; errors: FieldErrors; onChange: (keys: Keys) => void }) {
  const [visible, setVisible] = useState<number | null>(null);
  function change(index: number, patch: Partial<Keys[number]>) {
    onChange(draft.initialKeys.map((key, i) => i === index ? { ...key, ...patch } : key));
  }
  const props = (name: keyof FieldErrors) => ({ id: name, 'aria-invalid': !!errors[name], 'aria-describedby': errors[name] ? `${name}-hint` : undefined });
  const hint = (name: keyof FieldErrors) => errors[name] && <p id={`${name}-hint`} className="field-error">{errors[name]}</p>;
  return <>
    {draft.initialKeys.map((key, i) => {
      const id = `initial_key_${i}_id` as const, protocol = `initial_key_${i}_protocol` as const, secret = `initial_key_${i}_secret` as const;
      return <div className="initial-key-fields" key={i}>
        <div className="section-label"><h4>Key {i + 2}</h4><Button type="button" size="icon" variant="ghost" aria-label={`移除 Key ${i + 2}`} onClick={() => { setVisible(null); onChange(draft.initialKeys.filter((_, j) => j !== i)); }}><X size={15} /></Button></div>
        <div className="initial-key-options">
          <div className="field"><label htmlFor={id}>Key 名称</label><Input {...props(id)} value={key.id} onChange={event => change(i, { id: event.target.value })} autoComplete="off" />{hint(id)}</div>
          <div className="field"><label htmlFor={protocol}>Key 请求渠道</label><select {...props(protocol)} className="select-field" value={key.protocol} onChange={event => change(i, { protocol: event.target.value })}>{Object.entries(protocolName).map(([value, name]) => <option value={value} key={value}>{name}</option>)}</select>{hint(protocol)}</div>
        </div>
        <div className="field"><label htmlFor={secret}>API Key {i + 2}</label><div className="secret-input"><Input {...props(secret)} value={key.secret} type={visible === i && key.secret ? 'text' : 'password'} autoComplete="new-password" spellCheck={false} onChange={event => change(i, { secret: event.target.value })} />
          <button type="button" aria-label={`${visible === i ? '隐藏' : '显示'} API Key ${i + 2}`} onClick={() => setVisible(old => old === i ? null : i)}>{visible === i ? <EyeOff size={16} /> : <Eye size={16} />}</button></div>{hint(secret)}</div>
      </div>;
    })}
    {draft.initialKeys.length < MAX_PROVIDER_KEYS - 1 && <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => onChange([...draft.initialKeys, nextInitialKey(draft)])}><Plus size={14} />添加 Key</Button>}
    {hint('initialKeys')}
  </>;
}
