import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id:'files',createAPI,
  routes:[{path:'/',load:()=>import('./pages/browser.js')},{path:'/new/:operation',load:()=>import('./pages/new.js')}],
});
