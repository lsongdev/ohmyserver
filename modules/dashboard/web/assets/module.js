import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'dashboard', createAPI,
  routes: [{ path: '/', load: () => import('./pages/home.js') }],
});
