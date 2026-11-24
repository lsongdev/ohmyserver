import { createMount } from '@lsongdev/panel/mount.js';

export const mount = createMount({ routes: [
  { path: '/', load: () => import('./pages/checks.js') },
  { path: '/checks/:id', load: () => import('./pages/check.js') },
] });
