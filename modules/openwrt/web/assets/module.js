import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'openwrt', createAPI,
  routes: [
    { path: '/routers', load: () => import('./pages/routers.js') },
    { path: '/routers/new', load: () => import('./pages/router-form.js') },
    { path: '/routers/:id/edit', load: () => import('./pages/router-form.js') },
    { path: '/routers/:id', load: () => import('./pages/router-section.js') },
    ...['overview', 'network', 'dhcp', 'packages', 'firewall', 'services'].map(section => ({ path: `/routers/:id/${section}`, load: () => import('./pages/router-section.js'), props: { section } })),
  ],
});
