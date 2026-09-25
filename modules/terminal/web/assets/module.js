import { createAPI } from './api.js';
export default {
  id: 'terminal', createAPI,
  routes: [
    { path: '/local', props: { kind: 'local' }, load: () => import('./pages/session.js') },
    { path: '/ssh/:id', props: { kind: 'ssh' }, load: () => import('./pages/session.js') },
    { path: '/docker/:id', props: { kind: 'docker' }, load: () => import('./pages/session.js') },
  ],
};
