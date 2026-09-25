import { createAPI } from './api.js';

export default {
  id: 'forward', 
  createAPI,
  routes: [{ path: '/', load: () => import('./pages/listeners.js') }, { path: '/new', load: () => import('./pages/new.js') }],
};
