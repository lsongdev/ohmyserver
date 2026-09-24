import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'projects', title: '项目部署', icon: '▣', group: '容器与部署', entry: '/', navigation: [{ label: '项目', path: '/' }],
  createAPI,
  routes: [
    { path: '/', load: () => import('./pages/list.js') },
    { path: '/new', load: () => import('./pages/new.js') },
    { path: '/:id', load: () => import('./pages/detail.js') },
    { path: '/:id/deployments/:deploymentId', load: () => import('./pages/deployment.js') },
  ],
});
