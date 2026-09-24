import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'monitor', title: '主机监控', icon: '⌁', group: '概览', createAPI,
  entry: '/hosts', navigation: [{ label: '主机', path: '/hosts' }],
  routes: [
    { path: '/hosts', load: () => import('./pages/hosts.js') },
    { path: '/hosts/:hostname', load: () => import('./pages/host.js') },
  ],
});
