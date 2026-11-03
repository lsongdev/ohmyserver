// ZFS manager entry point (panel module contract, version 1). The
// host is a panel shell, or standalone.js on its own. Names of datasets and
// snapshots contain slashes, so pages take them in the query string.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/pools.js') },
  { path: '/pool', load: () => import('./pages/pool.js') },
  { path: '/pool/new', load: () => import('./pages/pool-new.js') },
  { path: '/dataset', load: () => import('./pages/dataset.js') },
  { path: '/dataset/new', load: () => import('./pages/dataset-new.js') },
  { path: '/dataset/schedule', load: () => import('./pages/schedule.js') },
  { path: '/snapshots', load: () => import('./pages/snapshots.js') },
  { path: '/snapshot', load: () => import('./pages/snapshot.js') },
];

export const mount = createMount({ routes });
