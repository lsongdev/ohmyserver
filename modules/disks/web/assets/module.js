import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'disks', title: '磁盘与存储', icon: '▤', group: '存储与系统', createAPI, entry: '/',
  navigation: [{ label: '磁盘', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/storage.js') }],
});
