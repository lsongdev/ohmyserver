// ZFS snapshot manager entry point (panel module contract, version 1). The
// host is a panel shell, or standalone.js on its own. Names of datasets and
// snapshots contain slashes, so pages take them in the query string.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/snapshots.js') },
  { path: '/snapshot', load: () => import('./pages/snapshot.js') },
  { path: '/datasets', load: () => import('./pages/datasets.js') },
  { path: '/dataset', load: () => import('./pages/dataset.js') },
];

export const mount = createMount({ routes });
