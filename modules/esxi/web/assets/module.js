import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'esxi', title: 'VMware ESXi', icon: '▤', group: '设备与 IoT', createAPI, entry: '/', navigation: [{ label: '主机', path: '/' }], routes: [{ path: '/', load: () => import('./pages/servers.js') }, { path: '/new', load: () => import('./pages/server-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/server-form.js') }] });
