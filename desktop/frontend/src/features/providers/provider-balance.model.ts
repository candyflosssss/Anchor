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
  }
}
