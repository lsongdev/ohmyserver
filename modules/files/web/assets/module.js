import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({
  id:'files',title:'文件管理',icon:'▱',group:'存储与系统',createAPI,entry:'/',navigation:[{label:'文件',path:'/'}],
  routes:[{path:'/',load:()=>import('./pages/browser.js')},{path:'/new/:operation',load:()=>import('./pages/new.js')}],
});
