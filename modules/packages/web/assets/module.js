import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'packages', createAPI,
  routes: [
    { path: '/', props: { view: 'installed' }, load: () => import('./pages/list.js') },
    { path: '/upgradable', props: { view: 'upgradable' }, load: () => import('./pages/list.js') },
  ],
});
