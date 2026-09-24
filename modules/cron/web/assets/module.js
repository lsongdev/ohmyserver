import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'cron', title: '定时任务', icon: '◷', group: '基础设施', entry: '/', createAPI,
  navigation: [{ label: '任务列表', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/jobs.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
