import { createAPI } from './api.js';

export default {
  id: 'docker',
  createAPI,
  routes: [
    { path: '/containers', load: () => import('./pages/containers.js') },
    { path: '/containers/new', load: () => import('./pages/create-resource.js'), props: { kind: 'container', title: '创建容器' } },
    { path: '/containers/:id', load: () => import('./pages/container.js') },
    { path: '/images', load: () => import('./pages/resources.js'), props: { kind: 'images', title: '镜像' } },
    { path: '/images/build', load: () => import('./pages/build-image.js') },
    { path: '/volumes', load: () => import('./pages/resources.js'), props: { kind: 'volumes', title: '存储卷' } },
    { path: '/volumes/new', load: () => import('./pages/create-resource.js'), props: { kind: 'volume', title: '创建存储卷' } },
    { path: '/networks', load: () => import('./pages/resources.js'), props: { kind: 'networks', title: '网络' } },
    { path: '/networks/new', load: () => import('./pages/create-resource.js'), props: { kind: 'network', title: '创建网络' } },
    { path: '/compose', load: () => import('./pages/compose.js') },
    { path: '/compose/new', load: () => import('./pages/compose-new.js') },
    { path: '/compose/:name', load: () => import('./pages/compose-detail.js') },
    { path: '/apps', load: () => import('./pages/apps.js') },
    { path: '/apps/:id', load: () => import('./pages/app.js') },
  ],
};
