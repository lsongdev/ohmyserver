import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'ssh', title: 'SSH 管理', icon: '⌘', group: '基础设施', createAPI, entry: '/servers', navigation: [{ label: '服务器', path: '/servers' }], routes: [{ path: '/servers', load: () => import('./pages/servers.js') }, { path: '/servers/new', load: () => import('./pages/server-form.js') }, { path: '/servers/:id/edit', load: () => import('./pages/server-form.js') }, { path: '/servers/:id/stats', load: () => import('./pages/stats.js') }] });
