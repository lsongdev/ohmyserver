import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'uptime', title: '在线监测', icon: '◉', group: '概览', 
  createAPI, entry: '/', navigation: [{ label: '监测目标', path: '/' }],
  routes: [{ path: '/', load: () => import('./pages/monitors.js') }, { path: '/new', load: () => import('./pages/new.js') }],
});
