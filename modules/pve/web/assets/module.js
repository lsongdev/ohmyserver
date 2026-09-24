import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'pve', title: 'Proxmox VE', icon: '▤', group: '设备与 IoT', createAPI, entry: '/', navigation: [{ label: '服务器', path: '/' }], routes: [{ path: '/', load: () => import('./pages/servers.js') }, { path: '/new', load: () => import('./pages/server-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/server-form.js') }, { path: '/:id/vms/new', load: () => import('./pages/vm-create.js') }] });
