import { createAPI } from './api.js';

export default {
  id: 'projects', navigation: [{ label: '项目', path: '/' }],
  createAPI,
  routes: [
    { path: '/', load: () => import('./pages/list.js') },
    { path: '/new', load: () => import('./pages/new.js') },
    { path: '/:id', load: () => import('./pages/detail.js') },
    { path: '/:id/deployments/:deploymentId', load: () => import('./pages/deployment.js') },
  ],
};
