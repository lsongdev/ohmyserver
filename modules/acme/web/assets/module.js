import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({
  id: 'acme', createAPI,
  routes: [
    { path: '/certificates', load: () => import('./pages/certificates.js') },
    { path: '/certificates/:id', load: () => import('./pages/certificate.js') },
    { path: '/certificates/new', load: () => import('./pages/issue.js') },
    { path: '/certificates/csr', load: () => import('./pages/csr.js') },
    { path: '/accounts', load: () => import('./pages/accounts.js') },
    { path: '/providers', load: () => import('./pages/providers.js') },
    { path: '/providers/new', load: () => import('./pages/provider-new.js') },
    { path: '/providers/:id/register', load: () => import('./pages/register.js') },
    { path: '/orders', load: () => import('./pages/orders.js') },
    { path: '/issuances', load: () => import('./pages/issuances.js') },
    { path: '/issuances/:id', load: () => import('./pages/issuance.js') },
  ],
});
