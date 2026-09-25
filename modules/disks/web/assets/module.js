import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'disks', createAPI,
  routes: [{ path: '/', load: () => import('./pages/storage.js') }],
});
