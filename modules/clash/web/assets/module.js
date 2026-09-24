import { createAPI } from './api.js';
export default {
  id: 'clash',
  title: 'Clash',
  icon: '◉',
  group: '网络服务',
  createAPI,
  entry: '/',
  navigation: [{ label: '代理实例', path: '/' }],
  routes: [
    { path: '/', load: () => import('./pages/servers.js') },
    { path: '/new', load: () => import('./pages/server-form.js') },
    { path: '/:id', load: () => import('./pages/detail.js') },
    { path: '/:id/edit', load: () => import('./pages/server-form.js') },
    { path: '/:id/proxies', load: () => import('./pages/proxies.js') },
    { path: '/:id/rules', load: () => import('./pages/rules.js') },
    { path: '/:id/connections', load: () => import('./pages/connections.js') },
    { path: '/:id/config', load: () => import('./pages/config.js') },
  ],
};
