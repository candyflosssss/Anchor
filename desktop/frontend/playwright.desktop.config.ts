import { defineConfig } from '@playwright/test';
import base from './playwright.config';
export default defineConfig({
  ...base,
  testIgnore: [], testMatch: ['desktop.spec.ts', 'chat.desktop.spec.ts', 'logs.spec.ts', 'agents.spec.ts', 'skills.spec.ts', 'proxies.spec.ts', 'provider-balances.spec.ts', 'connections.spec.ts', 'core-restart.spec.ts'],
  use: { ...base.use, baseURL: 'http://127.0.0.1:14202' },
  webServer: { command: 'npx vite --host 127.0.0.1 --mode desktop --port 14202', url: 'http://127.0.0.1:14202', reuseExistingServer: false, timeout: 30_000, gracefulShutdown: { signal: 'SIGTERM', timeout: 5000 } },
});
