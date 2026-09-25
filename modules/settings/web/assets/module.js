import { createAPI } from './api.js';
export default {
  id: 'settings', createAPI,
  routes: [{ path: '/', load: () => import('./pages/list.js') }, { path: '/new', load: () => import('./pages/new.js') }],
};
