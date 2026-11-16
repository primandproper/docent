import { svelte } from '@sveltejs/vite-plugin-svelte';
import wails from '@wailsio/runtime/plugins/vite';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// Tours are untrusted input, so the webview may load nothing but what the
// binary embeds: no remote scripts, images, frames or connections. Styles are
// inline because Svelte and Shiki both set style attributes. The policy is
// only injected into production builds: Vite's dev server needs a websocket.
const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self'",
  "connect-src 'self'",
  "frame-src 'none'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ');

function contentSecurityPolicy(): Plugin {
  return {
    name: 'docent-csp',
    apply: 'build',
    transformIndexHtml: () => [
      { tag: 'meta', attrs: { 'http-equiv': 'Content-Security-Policy', 'content': CSP }, injectTo: 'head-prepend' },
    ],
  };
}

export default defineConfig(({ mode }) => ({
  // `--mode mock` builds the app against a recorded backend, for browser tests.
  resolve: {
    alias: mode === 'mock' ? [{ find: /^\.\/backend$/, replacement: './backend.mock' }] : [],
  },
  publicDir: mode === 'mock' ? 'e2e/public' : 'public',
  build: {
    outDir: mode === 'mock' ? 'e2e/dist' : 'dist',
  },
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [svelte(), wails('./bindings'), contentSecurityPolicy()],
  test: {
    include: ['src/**/*.test.ts'],
  },
}));
