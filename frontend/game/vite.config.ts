import { defineConfig } from 'vite';

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://reverse-proxy';
const hmrClientPort = Number(process.env.VITE_HMR_CLIENT_PORT ?? 5173);

export default defineConfig({
  base: '/game/',
  server: {
    host: '0.0.0.0',
    port: 5173,
    cors: true,
    hmr: {
      protocol: 'ws',
      host: 'localhost',
      clientPort: hmrClientPort,
    },
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
