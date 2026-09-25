import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'cron', createAPI,
  routes: [{ path: '/', load: () => import('./pages/jobs.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
