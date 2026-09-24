import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'speedtest', title: '网络测速', icon: '◴', group: '网络服务', createAPI, entry: '/', navigation: [{ label: '测速记录', path: '/' }], routes: [{ path: '/', load: () => import('./pages/index.js') }] });
