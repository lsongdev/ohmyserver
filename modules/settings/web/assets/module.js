import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id: 'settings', title: '设置', icon: '⚙', group: '存储与系统', entry: '/', createAPI,
  navigation: [{ label: '全部设置', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/list.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
