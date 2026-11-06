// Notification center entry point (panel module contract, version 1). The host
// is a panel shell, or standalone.js on its own.
import { createMount } from '@lsongdev/panel/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/center.js') },
  { path: '/channels', load: () => import('./pages/channels.js') },
  { path: '/channels/new', load: () => import('./pages/channel-form.js') },
  { path: '/channels/:id', load: () => import('./pages/channel-form.js') },
  { path: '/deliveries', load: () => import('./pages/deliveries.js') },
];

export const mount = createMount({ routes });
