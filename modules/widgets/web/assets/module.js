import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({ id:'widgets',  createAPI,
  routes:[{path:'/',load:()=>import('./pages/list.js')},{path:'/new',load:()=>import('./pages/new.js')}] });
