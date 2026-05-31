import { defineConfig } from 'vite';

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://reverse-proxy';
const hmrClientPort = Number(process.env.VITE_HMR_CLIENT_PORT ?? 5173);
const hmrHost = process.env.VITE_HMR_HOST;
const hmrProtocol = process.env.VITE_HMR_PROTOCOL ?? 'ws';

export default defineConfig({
  base: '/game/',
  server: {
    host: '0.0.0.0',
    port: 5173,
    cors: true,
    hmr: {
      protocol: hmrProtocol,
      ...(hmrHost ? { host: hmrHost } : {}),
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
