import { defineModule } from '@panel/shared/module.js';
import { createAPI } from './api.js';
export default defineModule({ id:'ntp',title:'NTP 服务',icon:'◷',group:'网络服务',createAPI,entry:'/',navigation:[{label:'服务状态与配置',path:'/'}],routes:[{path:'/',load:()=>import('./pages/service.js')}] });
