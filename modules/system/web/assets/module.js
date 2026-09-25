import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'system', createAPI,
  routes: [{ path: '/', load: () => import('./pages/about.js') }],
});
