import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'packages', title: '软件包', icon: '□', group: '存储与系统', createAPI, entry: '/',
  navigation: [{ label: '已安装', path: '/' }, { label: '可升级', path: '/upgradable' }],
  routes: [
    { path: '/', props: { view: 'installed' }, load: () => import('./pages/list.js') },
    { path: '/upgradable', props: { view: 'upgradable' }, load: () => import('./pages/list.js') },
  ],
});
