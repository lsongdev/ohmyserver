import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';
import { PolicyForm } from '../forms.js';

const empty=()=>({name:'',max_body_bytes:0,request_timeout_seconds:0,rate_requests_per_second:0,rate_burst:0,rate_key:'client_ip',rate_max_keys:10000,rate_idle_timeout_seconds:600,concurrency_max:0});
const numeric=value=>({...value,max_body_bytes:Number(value.max_body_bytes),request_timeout_seconds:Number(value.request_timeout_seconds),rate_requests_per_second:Number(value.rate_requests_per_second),rate_burst:Number(value.rate_burst),rate_max_keys:Number(value.rate_max_keys),rate_idle_timeout_seconds:Number(value.rate_idle_timeout_seconds),concurrency_max:Number(value.concurrency_max)});
export default function Policies({api,navigate}){
 const [items,setItems]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
 const load=async(signal)=>setItems((await api.policies({signal}))||[]);
 useEffect(()=>{const c=new AbortController();load(c.signal).catch(e=>{if(e.name!=='AbortError')setError(e.message)});return()=>c.abort()},[api]);
 const run=async(fn)=>{setBusy(true);setError('');try{await fn();await load()}catch(e){setError(e.message||'操作失败')}finally{setBusy(false)}};
 return html`<section><div class="page-toolbar"><h2>Policies</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/webserver/policies/new')}>添加 Policy</button><button class="button" disabled=${busy} onClick=${()=>run(()=>load())}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
  <div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>最大请求体</th><th>超时</th><th>速率限制</th><th>并发限制</th><th>操作</th></tr></thead><tbody>${items.map(x=>html`<tr key=${x.id}><td>${x.name}</td><td>${x.max_body_bytes||'无限制'}</td><td>${x.request_timeout_seconds?`${x.request_timeout_seconds}s`:'默认'}</td><td>${x.rate_requests_per_second?`${x.rate_requests_per_second}/s · burst ${x.rate_burst} · ${x.rate_key}`:'未限制'}</td><td>${x.concurrency_max||'未限制'}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.deletePolicy(x.id))}>删除</button></td></tr><tr key=${`edit-${x.id}`}><td colspan="6"><details><summary>编辑 ${x.name}</summary><${PolicyForm} value=${x} busy=${busy} onSubmit=${value=>run(()=>api.updatePolicy(x.id,numeric(value)))}/></details></td></tr>`)}</tbody></table>${!items.length?html`<div class="empty-state">暂无 Policies</div>`:null}</div>
 </section>`;
}
