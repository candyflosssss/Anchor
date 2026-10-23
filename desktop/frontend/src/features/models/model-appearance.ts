import type { ModelGroup } from '../providers/provider-summary.model';

// A visual hint from model names, never a provider identity or routing decision.
const families = [
  ['deepseek', /(?:^|[^a-z0-9])(?:deepseek|ds)(?=$|[^a-z]|v\d)/i],
  ['gpt', /(?:^|[^a-z0-9])(?:gpt|chatgpt|codex|o[1-4])(?=$|[^a-z])/i],
  ['claude', /(?:^|[^a-z0-9])(?:claude|sonnet|opus|haiku)(?=$|[^a-z])/i],
  ['gemini', /(?:^|[^a-z0-9])gemini(?=$|[^a-z])/i],
  ['qwen', /(?:^|[^a-z0-9])qwen(?=$|[^a-z])/i],
] as const;

export function modelFamily(group: ModelGroup): string {
  const detect = (name: string) => families.find(([, pattern]) => pattern.test(name))?.[0];
  const alias = detect(group.alias);
  if (alias) return alias;
  const upstreams = group.candidates.map(c => detect(c.upstream));
  return upstreams.length && upstreams[0] && upstreams.every(family => family === upstreams[0]) ? upstreams[0] : 'neutral';
}
