import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id: 'dhcp', title: 'DHCP 服务', icon: '⇄', group: '网络服务', createAPI, entry: '/',
  navigation: [{ label: '配置', path: '/' }, { label: '动态租约', path: '/leases' }, { label: '静态租约', path: '/static-leases' }],
  routes: [
    { path: '/', load: () => import('./pages/overview.js') },
    { path: '/leases', load: () => import('./pages/leases.js') },
    { path: '/static-leases', load: () => import('./pages/leases.js').then(module => ({ default: module.StaticLeases })) },
    { path: '/static-leases/new', load: () => import('./pages/static-lease-new.js') },
  ],
});
