import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'forward', title: 'TCP/UDP 转发', icon: '⇢', group: '网络服务', 
  createAPI, entry: '/', navigation: [{ label: '转发监听器', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/listeners.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
