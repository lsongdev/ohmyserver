import { createAPI } from './api.js';
export default { id: 'esxi', createAPI,
  routes: [{ path: '/', load: () => import('./pages/servers.js') }, { path: '/new', load: () => import('./pages/server-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/server-form.js') }] };
