import { createAPI } from './api.js';

export default {
  id: 'disks', createAPI,
  routes: [{ path: '/', load: () => import('./pages/storage.js') }],
};
