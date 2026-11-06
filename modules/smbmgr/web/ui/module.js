// Samba manager entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from '@lsongdev/panel/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/shares.js') },
  { path: '/shares/new', load: () => import('./pages/share-form.js') },
  { path: '/shares/:name', load: () => import('./pages/share-form.js') },
  { path: '/users', load: () => import('./pages/users.js') },
  { path: '/sessions', load: () => import('./pages/sessions.js') },
  { path: '/logs', load: () => import('./pages/logs.js') },
  { path: '/global', load: () => import('./pages/global.js') },
];

export const mount = createMount({ routes });
