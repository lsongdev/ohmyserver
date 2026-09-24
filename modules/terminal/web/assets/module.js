import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id: 'terminal', createAPI, title: '终端', icon: '>_', group: '基础设施', 
  entry: '/local', navigation: [{ label: '本机终端', path: '/local' }],
  routes: [
    { path: '/local', props: { kind: 'local' }, load: () => import('./pages/session.js') },
    { path: '/ssh/:id', props: { kind: 'ssh' }, load: () => import('./pages/session.js') },
    { path: '/docker/:id', props: { kind: 'docker' }, load: () => import('./pages/session.js') },
  ],
});
