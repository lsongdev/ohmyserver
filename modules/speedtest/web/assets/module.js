import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'speedtest', createAPI,
  routes: [{ path: '/', load: () => import('./pages/index.js') }] });
