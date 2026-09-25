import { createAPI } from './api.js';
export default { id:'ntp',createAPI,
  routes:[{path:'/',load:()=>import('./pages/service.js')}] };
