// SSH terminal entry point (panel module contract, version 1). The host is
// a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/hosts.js') },
  { path: '/hosts/:id/terminal', load: () => import('./pages/terminal.js') },
  { path: '/key', load: () => import('./pages/key.js') },
];

export const mount = createMount({ routes });
