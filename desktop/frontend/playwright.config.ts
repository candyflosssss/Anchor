import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests', testIgnore: ['desktop.spec.ts', 'chat.desktop.spec.ts', 'logs.spec.ts', 'agents.spec.ts', 'provider-balances.spec.ts', 'connections.spec.ts'], fullyParallel: false, workers: 1, retries: 0,
  timeout: 30_000, reporter: 'list',
  use: { actionTimeout: 10_000, baseURL: 'http://127.0.0.1:14201', ...(process.env.ANCHOR_TEST_BROWSER_EXECUTABLE ? { launchOptions: { executablePath: process.env.ANCHOR_TEST_BROWSER_EXECUTABLE } } : { channel: 'chrome' }), viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce', trace: 'off' },
  webServer: { command: 'npm run dev -- --port 14201', url: 'http://127.0.0.1:14201', reuseExistingServer: false, timeout: 30_000, gracefulShutdown: { signal: 'SIGTERM', timeout: 5000 } },
});
