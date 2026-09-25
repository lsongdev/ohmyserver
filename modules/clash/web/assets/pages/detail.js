import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';
export default function Detail({api,params,navigate}){
 const[server,setServer]=useState(null);const[message,setMessage]=useState('');
 useEffect(()=>{const c=new AbortController();api.get(params.id,{signal:c.signal}).then(setServer).catch(e=>{if(e.name!=='AbortError')setMessage(e.message)});return()=>c.abort()},[api,params.id]);
 const test=async()=>{setMessage('测试中…');try{await api.test(params.id);setMessage('连接成功')}catch(e){setMessage(e.message)}};
 return html`<div><button class="button button-link" onclick=${()=>navigate('/')}>← Clash 实例</button>${message?html`<div class="notice-card"><p>${message}</p></div>`:null}${server?html`<section class="section"><div class="page-toolbar"><div><h2>${server.name}</h2><p>${server.api_url}</p></div><button class="button" onclick=${()=>navigate(`/${server.id}/edit`)}>编辑</button></div><p>${server.description||''}</p><p>控制密钥：${server.has_secret?'已配置':'未配置'}</p><button class="button" onclick=${test}>连接测试</button><h3>管理面板</h3><div class="button-group"><button class="button" onclick=${()=>navigate(`/${server.id}/proxies`)}>代理</button><button class="button" onclick=${()=>navigate(`/${server.id}/rules`)}>规则</button><button class="button" onclick=${()=>navigate(`/${server.id}/connections`)}>连接</button><button class="button" onclick=${()=>navigate(`/${server.id}/config`)}>运行配置</button></div></section>`:null}</div>`;
}
