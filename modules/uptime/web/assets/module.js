import { createAPI } from './api.js';

export default {
  id: 'uptime', 
  createAPI,
  routes: [{ path: '/', load: () => import('./pages/monitors.js') }, { path: '/new', load: () => import('./pages/new.js') }],
};
