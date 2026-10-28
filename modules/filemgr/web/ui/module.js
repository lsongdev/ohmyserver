// File manager entry point (panel module contract, version 1). The host is
// a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/browser.js') },
  { path: '/view', load: () => import('./pages/view.js') },
  { path: '/trash', load: () => import('./pages/trash.js') },
];

export const mount = createMount({ routes });
