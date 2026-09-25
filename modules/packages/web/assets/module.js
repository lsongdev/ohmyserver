import { createAPI } from './api.js';

export default {
  id: 'packages', createAPI,
  routes: [
    { path: '/', props: { view: 'installed' }, load: () => import('./pages/list.js') },
    { path: '/upgradable', props: { view: 'upgradable' }, load: () => import('./pages/list.js') },
  ],
};
