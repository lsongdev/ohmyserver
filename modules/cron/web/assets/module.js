import { createAPI } from './api.js';

export default {
  id: 'cron', createAPI,
  routes: [{ path: '/', load: () => import('./pages/jobs.js') }, { path: '/new', load: () => import('./pages/new.js') }],
};
