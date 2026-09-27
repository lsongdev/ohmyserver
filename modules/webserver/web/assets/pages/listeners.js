import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';
import { ListenerForm } from '../forms.js';

const blank = () => ({name:'',address:':8080',protocol:'http',enabled:true,min_tls_version:'TLS1.2',trusted_proxies:'',certificate_id:'',policy_ids:[]});
const payload = value => ({...value,certificate_id:value.certificate_id?Number(value.certificate_id):null});
export default function Listeners({api,navigate}) {
  const [items,setItems]=useState([]),[policies,setPolicies]=useState([]),[certificates,setCertificates]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=async(signal)=>{const [l,p,c]=await Promise.all([api.listeners({signal}),api.policies({signal}),api.certificates({signal})]);setItems(l||[]);setPolicies(p||[]);setCertificates(c||[])};
  useEffect(()=>{const c=new AbortController();load(c.signal).catch(e=>{if(e.name!=='AbortError')setError(e.message)});return()=>c.abort()},[api]);
  const run=async(fn)=>{setBusy(true);setError('');try{await fn();await load()}catch(e){setError(e.message||'操作失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>Listeners</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/listeners/new')}>添加 Listener</button><button class="button" disabled=${busy} onClick=${()=>run(()=>load())}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>监听地址</th><th>协议</th><th>启用</th><th>TLS</th><th>默认证书</th><th>策略</th><th>操作</th></tr></thead><tbody>${items.map(x=>html`<tr key=${x.id}><td>${x.name}</td><td>${x.address}</td><td>${x.protocol}</td><td>${x.enabled?'是':'否'}</td><td>${x.min_tls_version||'—'}</td><td>${x.certificate_name||'—'}</td><td>${(x.policy_names||[]).join(', ')||'—'}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.deleteListener(x.id))}>删除</button></td></tr><tr key=${`edit-${x.id}`}><td colspan="8"><details><summary>编辑 ${x.name}</summary><${ListenerForm} value=${{...x,certificate_id:x.certificate_id||'',policy_ids:x.policy_ids||[]}} policies=${policies} certificates=${certificates} busy=${busy} onSubmit=${value=>run(()=>api.updateListener(x.id,payload(value)))}/></details></td></tr>`)}</tbody></table>${!items.length?html`<div class="empty-state">暂无 Listeners</div>`:null}</div>
  </section>`;
}
