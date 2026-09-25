import { createAPI } from './api.js';
export default {
  id:'files',createAPI,
  routes:[{path:'/',load:()=>import('./pages/browser.js')},{path:'/new/:operation',load:()=>import('./pages/new.js')}],
};
