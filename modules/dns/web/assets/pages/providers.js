import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';

export default function Providers({api,href,navigate}){
  const [items,setItems]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>api.listProviders().then(setItems).catch(err=>setError(err.message||'DNS 服务商读取失败'));
  useEffect(()=>{const controller=new AbortController();api.listProviders({signal:controller.signal}).then(setItems).catch(err=>{if(err.name!=='AbortError')setError(err.message||'DNS 服务商读取失败')});return()=>controller.abort()},[api]);
  const remove=async item=>{if(!confirm(`删除 DNS 服务商 ${item.name}？`))return;setBusy(true);setError('');try{await api.deleteProvider(item.id);await load()}catch(err){setError(err.message||'删除失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>DNS 服务商连接</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/dns/providers/new')}>添加连接</button><button class="button" disabled=${busy} onClick=${load}>刷新</button></div></div>
    ${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>类型</th><th>创建时间</th><th>操作</th></tr></thead><tbody>${items.map(item=>html`<tr key=${item.id}><td><a href=${href(`/dns/providers/${item.id}`)}>${item.name}</a></td><td>${item.type}</td><td>${item.created_at}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>remove(item)}>删除</button></td></tr>`)}</tbody></table>${!items.length&&!error?html`<div class="empty-state">暂无 DNS 服务商连接</div>`:null}</div>
  </section>`;
}
