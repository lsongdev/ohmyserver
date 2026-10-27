// CI entry point (panel module contract, version 1). The host is a panel
// shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/repos.js') },
  { path: '/repos/:id', load: () => import('./pages/repo.js') },
  { path: '/builds', load: () => import('./pages/builds.js') },
  { path: '/builds/:id', load: () => import('./pages/build.js') },
];

export const mount = createMount({ routes });
