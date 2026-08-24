import { fileURLToPath, URL } from 'node:url';
import { copyFile, mkdir } from 'node:fs/promises';
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react(), {
    name: 'bundle-offline-gamehub-sdk',
    async closeBundle() {
      const target = fileURLToPath(new URL('./dist/sdk/', import.meta.url));
      await mkdir(target, { recursive: true });
      await Promise.all([
        copyFile(fileURLToPath(new URL('../sdk/gamehub-js/dist/index.js', import.meta.url)), `${target}/gamehub-js.js`),
        copyFile(fileURLToPath(new URL('../sdk/gamehub-js/dist/index.d.ts', import.meta.url)), `${target}/gamehub-js.d.ts`),
      ]);
    },
  }],
  resolve: {
    alias: {
      '@igame/gamehub-js': fileURLToPath(new URL('../sdk/gamehub-js/src/index.ts', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
  },
  server: {
    port: 5173,
    proxy: {
      // The dev server is a different origin from the API, so the browser's
      // Origin would not match the service address and every state-changing
      // request would be refused. Presenting the target's own origin keeps the
      // dev proxy working without loosening the check on the server, and never
      // ships: production serves the built bundle from the same origin.
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        headers: { Origin: 'http://localhost:8080' },
      },
      '/mcp': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        headers: { Origin: 'http://localhost:8080' },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
});
