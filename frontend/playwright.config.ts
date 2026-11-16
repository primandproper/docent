import { defineConfig, devices } from '@playwright/test';

const PORT = 9321;

// The tests run the production bundle, built against a recorded backend, in
// WebKit: the same engine as the WKWebView the app ships in.
export default defineConfig({
  testDir: 'e2e',
  outputDir: 'e2e/results',
  fullyParallel: true,
  // Each page loads Mermaid, Shiki and ELK. More than a few at once and they starve each other.
  workers: 4,
  forbidOnly: !!process.env.CI,
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    viewport: { width: 1440, height: 900 },
  },
  projects: [
    {
      name: 'webkit',
      testIgnore: /perf\.spec\.ts/,
      use: { ...devices['Desktop Safari'], viewport: { width: 1440, height: 900 } },
    },
    // The performance checks are stopwatches. They run after everything else
    // has finished, one at a time, so they time the app and not their neighbours.
    {
      name: 'perf',
      testMatch: /perf\.spec\.ts/,
      dependencies: ['webkit'],
      use: { ...devices['Desktop Safari'], viewport: { width: 1440, height: 900 } },
    },
  ],
  webServer: {
    command: `pnpm exec vite preview --mode mock --outDir e2e/dist --host 127.0.0.1 --port ${PORT} --strictPort`,
    url: `http://127.0.0.1:${PORT}`,
    reuseExistingServer: !process.env.CI,
  },
});
