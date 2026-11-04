// Network manager entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/interfaces.js') },
  { path: '/interfaces/:name', load: () => import('./pages/interface.js') },
  { path: '/interfaces/:name/settings', load: () => import('./pages/settings.js') },
  { path: '/routes', load: () => import('./pages/routes.js') },
  { path: '/ports', load: () => import('./pages/ports.js') },
  { path: '/tools', load: () => import('./pages/tools.js') },
];

export const mount = createMount({ routes });
