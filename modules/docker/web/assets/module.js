import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'docker', createAPI,
  routes: [
    { path: '/containers', load: () => import('./pages/containers.js') },
    { path: '/containers/new', load: () => import('./pages/create-resource.js'), props: { kind: 'container', load: () => import('./pages/container.js') },
    { path: '/images', load: () => import('./pages/resources.js'), props: { kind: 'images', load: () => import('./pages/build-image.js') },
    { path: '/volumes', load: () => import('./pages/resources.js'), props: { kind: 'volumes', load: () => import('./pages/create-resource.js'), props: { kind: 'volume', load: () => import('./pages/resources.js'), props: { kind: 'networks', load: () => import('./pages/create-resource.js'), props: { kind: 'network', load: () => import('./pages/compose.js') },
    { path: '/compose/new', load: () => import('./pages/compose-new.js') },
    { path: '/compose/:name', load: () => import('./pages/compose-detail.js') },
    { path: '/apps', load: () => import('./pages/apps.js') },
    { path: '/apps/:id', load: () => import('./pages/app.js') },
  ],
});
