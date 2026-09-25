import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';
import { HostForm, RouteForm } from '../forms.js';

const emptyHost=()=>({name:'',hostnames:'',enabled:true,certificate_id:'',force_https:false,listener_ids:[],policy_ids:[]});
const emptyRoute=()=>({name:'',enabled:true,method:'',path:'/',handler_type:'reverse_proxy',upstream_url:'',policy_ids:[],request_timeout_seconds:0,response_header_timeout_seconds:0,max_conns_per_host:0,response_status:200,redirect_status:302});
const hostPayload=value=>({...value,certificate_id:value.certificate_id?Number(value.certificate_id):null});
const routePayload=value=>({...value,request_timeout_seconds:Number(value.request_timeout_seconds),response_header_timeout_seconds:Number(value.response_header_timeout_seconds),max_conns_per_host:Number(value.max_conns_per_host),response_status:Number(value.response_status),redirect_status:Number(value.redirect_status)});
export default function Hosts({api,navigate}){
 const [hosts,setHosts]=useState([]),[listeners,setListeners]=useState([]),[policies,setPolicies]=useState([]),[certificates,setCertificates]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
 const load=async(signal)=>{const [h,l,p,c]=await Promise.all([api.hosts({signal}),api.listeners({signal}),api.policies({signal}),api.certificates({signal})]);setHosts(h||[]);setListeners(l||[]);setPolicies(p||[]);setCertificates(c||[])};
 useEffect(()=>{const c=new AbortController();load(c.signal).catch(e=>{if(e.name!=='AbortError')setError(e.message)});return()=>c.abort()},[api]);
 const run=async(fn)=>{setBusy(true);setError('');try{await fn();await load()}catch(e){setError(e.message||'操作失败')}finally{setBusy(false)}};
 return html`<section><div class="page-toolbar"><h2>Virtual Hosts</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/hosts/new')}>添加 Virtual Host</button><button class="button" disabled=${busy} onClick=${()=>run(()=>load())}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
  <div class="web-host-list">${hosts.map(host=>html`<article class="section" key=${host.id}><div class="page-toolbar"><div><h3>${host.name}</h3><div>${host.hostnames}</div></div><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.deleteHost(host.id))}>删除 Host</button></div><div class="web-host-meta">${host.enabled?'启用':'停用'} · ${host.force_https?'强制 HTTPS':'不强制 HTTPS'} · 证书：${host.certificate_name||'Listener 默认'} · Listeners：${(host.listener_names||[]).join(', ')||'—'} · Policies：${(host.policy_names||[]).join(', ')||'—'}</div>
   <details><summary>编辑 Host</summary><${HostForm} value=${{...host,certificate_id:host.certificate_id||'',listener_ids:host.listener_ids||[],policy_ids:host.policy_ids||[]}} listeners=${listeners} policies=${policies} certificates=${certificates} busy=${busy} onSubmit=${value=>run(()=>api.updateHost(host.id,hostPayload(value)))}/></details>
   <div class="data-table-wrap"><table class="data-table"><thead><tr><th>Route</th><th>匹配</th><th>处理器</th><th>目标</th><th>状态</th><th>操作</th></tr></thead><tbody>${(host.routes||[]).map(route=>html`<tr key=${route.id}><td>${route.name}</td><td>${route.method||'*'} ${route.path}</td><td>${route.handler_type}</td><td class="mono">${route.upstream_url||route.root||route.redirect_location||'—'}</td><td>${route.enabled?'启用':'停用'}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.deleteRoute(route.id))}>删除</button></td></tr><tr key=${`edit-${route.id}`}><td colspan="6"><details><summary>编辑 ${route.name}</summary><${RouteForm} value=${{...route,policy_ids:route.policy_ids||[]}} policies=${policies} busy=${busy} onSubmit=${value=>run(()=>api.updateRoute(route.id,routePayload(value)))}/></details></td></tr>`)}</tbody></table></div>
   <div class="page-toolbar"><h4>Routes</h4><button class="button button-primary" onClick=${()=>navigate(`/hosts/${host.id}/routes/new`)}>添加 Route</button></div>
  </article>`)}</div>${!hosts.length?html`<div class="empty-state">暂无 Virtual Hosts</div>`:null}
 </section>`;
}
