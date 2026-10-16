import { Circle, CircleHelp, CircleDot, Minus, Rocket, Zap, Check, TriangleAlert, type LucideIcon } from 'lucide-react';
import type { RequestLog } from './log.model';
import { accelerationSummary, tierSnapshot, type TierKind, type TierSnapshot } from './log-service-tier.model';

const icons: Record<TierKind, LucideIcon> = { fast: Zap, ultrafast: Rocket, standard: Circle, auto: CircleDot, other: CircleDot, unknown: CircleHelp, none: Minus };
export function TierValue({ tier, state, note }: { tier: TierSnapshot; state?: string; note?: string }) {
  const Icon = icons[tier.kind];
  return <div className="log-tier" data-tier={tier.kind}>
    <code className="log-tier-raw">{tier.raw}</code>
    <div className="log-tier-caption">
      <span className="log-tier-badge"><Icon size={14} aria-hidden="true" /><span>{tier.label}</span></span>
      {state === 'confirmed' && <Check size={13} className="log-tier-confirmed" aria-hidden="true" />}
      {state === 'mismatch' && <TriangleAlert size={13} className="log-tier-warning" aria-hidden="true" />}
    </div>
    {note && <small className={state === 'mismatch' ? 'log-tier-warning' : 'log-tier-note'}>{note}</small>}
  </div>;
}
export function SentTier({ row }: { row: RequestLog }) {
  return <TierValue tier={tierSnapshot(row.evidence, 'sent')} />;
}
export function Acceleration({ row }: { row: RequestLog }) {
  const { tier, state, label } = accelerationSummary(row.evidence);
  return <TierValue tier={tier} state={state} note={label} />;
}
