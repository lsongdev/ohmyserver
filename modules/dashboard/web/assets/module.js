import { createAPI } from './api.js';

export default {
  id: 'dashboard', createAPI,
  routes: [{ path: '/', load: () => import('./pages/home.js') }],
};
