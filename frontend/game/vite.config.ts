import { defineConfig } from 'vite';

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET ?? 'http://reverse-proxy';
const hmrClientPort = Number(process.env.VITE_HMR_CLIENT_PORT ?? 5173);
const hmrHost = process.env.VITE_HMR_HOST;
const hmrProtocol = process.env.VITE_HMR_PROTOCOL ?? 'ws';
const allowedHosts = parseAllowedHosts(process.env.VITE_ALLOWED_HOSTS);

function parseAllowedHosts(value: string | undefined) {
  if (!value) return undefined;
  if (value.trim().toLowerCase() === 'true') return true;

  const hosts = value
    .split(',')
    .map((host) => host.trim())
    .filter(Boolean);

  return hosts.length > 0 ? hosts : undefined;
}

export default defineConfig({
  base: '/game/',
  server: {
    host: '0.0.0.0',
    port: 5173,
    ...(allowedHosts ? { allowedHosts } : {}),
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
  preview: {
    host: '0.0.0.0',
    port: 5173,
    ...(allowedHosts ? { allowedHosts } : {}),
  },
});
