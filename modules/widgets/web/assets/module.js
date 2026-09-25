import { createAPI } from './api.js';

export default { id:'widgets',  createAPI,
  routes:[{path:'/',load:()=>import('./pages/list.js')},{path:'/new',load:()=>import('./pages/new.js')}] };
