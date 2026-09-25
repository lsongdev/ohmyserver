import { createAPI } from './api.js';

export default {
  id: 'system', createAPI,
  routes: [{ path: '/', load: () => import('./pages/about.js') }],
};
