import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'monitor', createAPI,
  routes: [
    { path: '/hosts', load: () => import('./pages/hosts.js') },
    { path: '/hosts/:hostname', load: () => import('./pages/host.js') },
  ],
});
