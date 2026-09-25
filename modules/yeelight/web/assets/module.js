import { createAPI } from './api.js';
export default { id: 'yeelight', createAPI,
  routes: [{ path: '/', load: () => import('./pages/devices.js') }, { path: '/new', load: () => import('./pages/device-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/device-form.js') }] };
