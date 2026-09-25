import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'forward', 
  createAPI,
  routes: [{ path: '/', load: () => import('./pages/listeners.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
