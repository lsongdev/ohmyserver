import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'yeelight', title: 'Yeelight', icon: '☼', group: '设备与 IoT', createAPI, entry: '/', navigation: [{ label: '智能灯具', path: '/' }], routes: [{ path: '/', load: () => import('./pages/devices.js') }, { path: '/new', load: () => import('./pages/device-form.js') }, { path: '/:id', load: () => import('./pages/detail.js') }, { path: '/:id/edit', load: () => import('./pages/device-form.js') }] });
