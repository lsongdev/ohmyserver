import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id: 'webserver', createAPI,
  routes: [{path:'/listeners',load:()=>import('./pages/listeners.js')},{path:'/listeners/new',load:()=>import('./pages/listener-new.js')},{path:'/policies',load:()=>import('./pages/policies.js')},{path:'/policies/new',load:()=>import('./pages/policy-new.js')},{path:'/hosts',load:()=>import('./pages/hosts.js')},{path:'/hosts/new',load:()=>import('./pages/host-new.js')},{path:'/hosts/:id/routes/new',load:()=>import('./pages/route-new.js')}] });
