import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';

export default function Zone({api,params,navigate}){
  const [zone,setZone]=useState(null),[records,setRecords]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>Promise.all([api.getZone(params.id),api.listRecords(params.id)]).then(([z,r])=>{setZone(z);setRecords(r)}).catch(err=>setError(err.message||'区域记录读取失败'));
  useEffect(()=>{const controller=new AbortController();Promise.all([api.getZone(params.id),api.listRecords(params.id)]).then(([z,r])=>{setZone(z);setRecords(r)}).catch(err=>{if(err.name!=='AbortError')setError(err.message||'区域记录读取失败')});return()=>controller.abort()},[api,params.id]);
  const remove=async item=>{if(!confirm(`删除 ${item.name} ${item.type} 记录？`))return;setBusy(true);setError('');try{await api.deleteRecord(item.id);await load()}catch(err){setError(err.message||'删除失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>${zone?.name||'区域'}</h2><div class="button-group"><button class="button button-primary" disabled=${!zone} onClick=${()=>navigate(`/zones/${params.id}/records/new`)}>添加记录</button><button class="button" disabled=${busy} onClick=${load}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>类型</th><th>TTL</th><th>值</th><th>操作</th></tr></thead><tbody>${records.map(item=>html`<tr key=${item.id}><td>${item.name}</td><td>${item.type}</td><td>${item.ttl}</td><td class="mono">${item.value}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>remove(item)}>删除</button></td></tr>`)}</tbody></table>${!records.length&&!error?html`<div class="empty-state">暂无解析记录</div>`:null}</div>
  </section>`;
}
