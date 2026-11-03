// Notification center entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/center.js') },
];

export const mount = createMount({ routes });
