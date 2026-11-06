// Package manager entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from '@lsongdev/panel/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/packages.js') },
  { path: '/recommended', load: () => import('./pages/recommended.js') },
  { path: '/jobs', load: () => import('./pages/jobs.js') },
  { path: '/jobs/:id', load: () => import('./pages/job.js') },
];

export const mount = createMount({ routes });
