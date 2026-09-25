import { createAPI } from './api.js';
export default { id: 'pve', createAPI,
  routes: [{ path: '/', load: () => import('./pages/servers.js') }, { path: '/new', load: () => import('./pages/server-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/server-form.js') }, { path: '/:id/vms/new', load: () => import('./pages/vm-create.js') }] };
