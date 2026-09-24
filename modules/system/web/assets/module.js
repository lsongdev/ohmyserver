import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'system', title: '系统信息', icon: 'ⓘ', group: '存储与系统', entry: '/', createAPI,
  navigation: [{ label: '关于', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/about.js') }],
});
