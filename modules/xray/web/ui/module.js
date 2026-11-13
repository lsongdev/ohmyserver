import { createMount } from '@lsongdev/panel/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/overview.js') },
  { path: '/config', load: () => import('./pages/editor.js') },
  { path: '/:section', load: () => import('./pages/list.js') },
  { path: '/:section/new', load: () => import('./pages/editor.js') },
  { path: '/:section/:index/edit', load: () => import('./pages/editor.js') },
];

export const mount = createMount({ routes });
