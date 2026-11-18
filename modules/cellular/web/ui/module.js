import { createMount } from '@lsongdev/panel/mount.js';

export const mount = createMount({
  routes: [
    { path: '/', load: () => import('./status.js') },
    { path: '/sms', load: () => import('./sms.js') },
    { path: '/at', load: () => import('./at.js') },
  ],
  envelope: false,
});
