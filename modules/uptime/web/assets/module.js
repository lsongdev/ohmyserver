import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'uptime', 
  createAPI,
  routes: [{ path: '/', load: () => import('./pages/monitors.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
