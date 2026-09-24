import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';

export default defineModule({ id:'widgets', title:'首页组件', icon:'▦', group:'存储与系统',  createAPI, entry:'/', navigation:[{label:'组件配置',path:'/'}], routes:[{path:'/',load:()=>import('./pages/list.js')},{path:'/new',load:()=>import('./pages/new.js')}] });
