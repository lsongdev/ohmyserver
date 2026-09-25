import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id:'ntp',createAPI,
  routes:[{path:'/',load:()=>import('./pages/service.js')}] });
