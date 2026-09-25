import { createAPI } from './api.js';
export default { id: 'ssh', createAPI,
  routes: [{ path: '/servers', load: () => import('./pages/servers.js') }, { path: '/servers/new', load: () => import('./pages/server-form.js') }, { path: '/servers/:id/edit', load: () => import('./pages/server-form.js') }, { path: '/servers/:id/stats', load: () => import('./pages/stats.js') }] };
