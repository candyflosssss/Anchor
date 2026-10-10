import { MockProviderTransport, type Scenario } from '../features/providers/provider.mock';
import type { ProviderTransport } from '../features/providers/provider.model';
import { DesktopProviderTransport } from './transport';
export const prototype = import.meta.env.MODE === 'prototype';
export interface ProviderEnvironment { transport: ProviderTransport; mock: boolean; reset?: (value: Scenario) => void }
export function createEnvironment(): ProviderEnvironment {
  if (prototype) {
    const transport = new MockProviderTransport();
    return { transport, mock: true, reset: value => transport.reset(value) };
  }
  return { transport: new DesktopProviderTransport(), mock: false };
}
