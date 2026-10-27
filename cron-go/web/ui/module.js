// Job scheduler entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/jobs.js') },
  { path: '/jobs/:id', load: () => import('./pages/job.js') },
  { path: '/runs/:id', load: () => import('./pages/run.js') },
];

export const mount = createMount({ routes });
