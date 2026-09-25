import { createAPI } from './api.js';

export default {
  id: 'monitor', createAPI,
  routes: [
    { path: '/hosts', load: () => import('./pages/hosts.js') },
    { path: '/hosts/:hostname', load: () => import('./pages/host.js') },
  ],
};
