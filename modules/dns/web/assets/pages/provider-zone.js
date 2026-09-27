import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';
import { decodeProviderZone } from '../provider-route.js';

export default function ProviderZone({api,params,href,navigate}){
  const [zoneID,setZoneID]=useState(''),[zoneName,setZoneName]=useState(''),[records,setRecords]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  useEffect(()=>{
    let decoded;
    try{decoded=decodeProviderZone(params.zone)}catch{setError('托管区域地址无效');return}
    setZoneID(decoded);
    const c=new AbortController();
    Promise.all([api.listProviderZones(params.id,{signal:c.signal}),api.listProviderRecords(params.id,decoded,{signal:c.signal})]).then(([zones,items])=>{
      setZoneName((zones||[]).find(item=>item.id===decoded)?.name||decoded);setRecords(items||[])
    }).catch(err=>{if(err.name!=='AbortError')setError(err.message||'DNS 记录读取失败')});
    return()=>c.abort();
  },[api,params.id,params.zone]);
  const load=async()=>{const items=await api.listProviderRecords(params.id,zoneID);setRecords(items||[])};
  const run=async(fn)=>{setBusy(true);setError('');try{await fn();await load()}catch(err){setError(err.message||'操作失败')}finally{setBusy(false)}};
  const remove=item=>{if(!confirm(`删除 ${item.name} ${item.type} 记录？`))return;run(()=>api.deleteProviderRecord(params.id,zoneID,item.id))};
  if(!zoneID)return html`<section><div class="notice-card error" role="alert">${error||'托管区域地址无效'}</div></section>`;
  return html`<section><div class="page-toolbar"><div><a href=${href(`/providers/${params.id}`)}>托管区域</a><h2>${zoneName}</h2><div class="mono">${zoneID}</div></div><div class="button-group"><button class="button button-primary" disabled=${busy} onClick=${()=>navigate(`/providers/${params.id}/zones/${params.zone}/records/new`)}>添加记录</button><button class="button" disabled=${busy} onClick=${()=>run(load)}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>类型</th><th>值</th><th>TTL</th><th>修改时间</th><th>操作</th></tr></thead><tbody>${records.map(item=>html`<tr key=${item.id}><td>${item.name}</td><td>${item.type}</td><td class="mono">${item.value}</td><td>${item.ttl}</td><td>${item.updated_at||'—'}</td><td><div class="button-group"><button class="button" disabled=${busy} onClick=${()=>navigate(`/providers/${params.id}/zones/${params.zone}/records/${encodeURIComponent(item.id)}/edit`)}>编辑</button><button class="button button-danger" disabled=${busy} onClick=${()=>remove(item)}>删除</button></div></td></tr>`)}</tbody></table>${!records.length&&!error?html`<div class="empty-state">该区域没有 DNS 记录</div>`:null}</div>
  </section>`;
}
