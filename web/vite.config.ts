import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 5173,
    // A tunnel serves the dev server under a hostname Vite has never seen, and
    // Vite blocks unknown hosts by default. Both providers get an allowance so
    // `cloudflared tunnel --url http://localhost:5173` just works.
    allowedHosts: ['.trycloudflare.com', '.ngrok-free.app', '.ngrok.io'],
    proxy: {
      // Same-origin in development too, so the API client needs no base URL and
      // no CORS is involved on the happy path.
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // Uploaded photos. The API hands out addresses under /media, so without
      // this the <img> asks the dev server for a path it knows nothing about
      // and every avatar renders broken.
      '/media': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    // Caddy serves this directory in production — see the root Caddyfile.
    outDir: 'dist',
    sourcemap: true,
  },
});
