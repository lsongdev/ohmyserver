import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'dashboard', title: '系统概览', icon: '⌂', group: '概览', entry: '/', createAPI,
  navigation: [{ label: '总览', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/home.js') }],
});
