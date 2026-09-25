import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id: 'dhcp', createAPI,
  routes: [
    { path: '/', load: () => import('./pages/overview.js') },
    { path: '/leases', load: () => import('./pages/leases.js') },
    { path: '/static-leases', load: () => import('./pages/leases.js').then(module => ({ default: module.StaticLeases })) },
    { path: '/static-leases/new', load: () => import('./pages/static-lease-new.js') },
  ],
});
