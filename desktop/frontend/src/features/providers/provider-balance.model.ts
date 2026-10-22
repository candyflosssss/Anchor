import type { Provider } from './provider.model';
import type { AccountObservation } from './provider-account.model';

export interface ProviderBalance {
  observation?: AccountObservation;
  error?: string;
  failure?: string;
  refreshing?: boolean;
}

/** One connected instance owns this in-memory cache, including across page unmounts. */
export class ProviderBalanceCache {
  constructor(readonly instanceId: string) {}
  private readonly entries = new Map<string, { revision: number; value: ProviderBalance }>();
  private readonly toggles = new Map<string, { revision: number; queryRevision: number }>();

  /** Only a confirmed enable-only save can carry observations across revisions. */
  retainToggle(before: Provider, after: Provider) {
    if (before.id !== after.id || after.revision !== before.revision + 1 || before.enabled === after.enabled ||
      before.homepage_url !== after.homepage_url || before.protocol !== after.protocol || before.has_secret !== after.has_secret ||
      before.timeout_seconds !== after.timeout_seconds || before.first_token_timeout_seconds !== after.first_token_timeout_seconds || before.max_concurrent !== after.max_concurrent ||
      before.cost_multiplier !== after.cost_multiplier || JSON.stringify(before.base_urls) !== JSON.stringify(after.base_urls)) return;
    this.toggles.set(after.id, { revision: after.revision, queryRevision: this.queryRevision(before) });
    const previous = this.read(before);
    if (previous) this.write(after, previous);
  }

  queryRevision(provider: Provider): number {
    const toggle = this.toggles.get(provider.id);
    return toggle?.revision === provider.revision ? toggle.queryRevision : provider.revision;
  }

  read(provider: Provider): ProviderBalance | undefined {
    const cached = this.entries.get(provider.id);
    return cached?.revision === provider.revision ? cached.value : undefined;
  }

  write(provider: Provider, value: ProviderBalance) {
    this.entries.set(provider.id, { revision: provider.revision, value });
  }

  snapshot(providers: Provider[]): Record<string, ProviderBalance> {
    const entries: Record<string, ProviderBalance> = {};
    for (const provider of providers) {
      const value = this.read(provider);
      if (value) entries[provider.id] = value;
    }
    return entries;
  }

  prune(providers: Provider[]) {
    const revisions = new Map(providers.map(provider => [provider.id, provider.revision]));
    for (const [id, cached] of this.entries) {
      if (revisions.get(id) !== cached.revision) this.entries.delete(id);
    }
    for (const [id, toggle] of this.toggles) {
      if (revisions.get(id) !== toggle.revision) this.toggles.delete(id);
    }
  }
}
