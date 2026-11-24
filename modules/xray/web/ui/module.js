import { createMount } from '@lsongdev/panel/mount.js';

const routes = [
  { path: '/', load: () => import('./pages/overview.js') },
  { path: '/config', load: () => import('./pages/editor.js') },
  { path: '/outbounds/subscriptions', load: () => import('./pages/subscription-list-redirect.js') },
  { path: '/outbounds/subscriptions/new', load: () => import('./pages/subscription-list-redirect.js').then(module => ({ default: module.AddSubscriptionRedirect })) },
  { path: '/outbounds/subscriptions/:id/edit', load: () => import('./pages/subscription-form.js') },
  { path: '/outbounds/subscriptions/:id', load: () => import('./pages/subscription.js') },
  { path: '/:section', load: () => import('./pages/list.js') },
  { path: '/:section/new', load: () => import('./pages/editor.js') },
  { path: '/:section/:index/edit', load: () => import('./pages/editor.js') },
];

export const mount = createMount({ routes });
