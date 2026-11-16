import { defineConfig, devices } from '@playwright/test';

const PORT = 9321;

// The tests run the production bundle, built against a recorded backend, in
// WebKit: the same engine as the WKWebView the app ships in.
export default defineConfig({
  testDir: 'e2e',
  outputDir: 'e2e/results',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    viewport: { width: 1440, height: 900 },
  },
  projects: [{ name: 'webkit', use: { ...devices['Desktop Safari'], viewport: { width: 1440, height: 900 } } }],
  webServer: {
    command: `pnpm exec vite preview --mode mock --outDir e2e/dist --host 127.0.0.1 --port ${PORT} --strictPort`,
    url: `http://127.0.0.1:${PORT}`,
    reuseExistingServer: !process.env.CI,
  },
});
