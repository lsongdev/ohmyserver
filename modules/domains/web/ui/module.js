// Domains entry point (panel module contract, version 1). The host is a
// panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/zones.js') },
  { path: '/zones/:account/:zone', load: () => import('./pages/zone.js') },
  { path: '/accounts', load: () => import('./pages/accounts.js') },
  { path: '/accounts/new', load: () => import('./pages/account.js') },
  { path: '/accounts/:id', load: () => import('./pages/account.js') },
];

export const mount = createMount({ routes });
