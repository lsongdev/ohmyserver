import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'ipmi', createAPI,
  routes: [{ path: '/', load: () => import('./pages/servers.js') }, { path: '/new', load: () => import('./pages/server-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/server-form.js') }] });
