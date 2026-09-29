import { html } from '@module/ui';
import { useEffect,useState } from 'preact/hooks';
import { encodeProviderZone } from '../provider-route.js';

export default function Provider({api,params,href}){
  const [zones,setZones]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=signal=>api.listProviderZones(params.id,{signal}).then(setZones).catch(err=>{if(err.name!=='AbortError')setError(err.message||'托管区域读取失败')});
  useEffect(()=>{const c=new AbortController();load(c.signal);return()=>c.abort()},[api,params.id]);
  const refresh=async()=>{setBusy(true);setError('');try{await load()}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><div><a href=${href('/providers')}>DNS 服务商</a><h2>托管区域</h2></div><button class="button" disabled=${busy} onClick=${refresh}>刷新</button></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>区域</th><th>区域 ID</th><th>创建时间</th><th></th></tr></thead><tbody>${zones.map(zone=>html`<tr key=${zone.id}><td>${zone.name}</td><td class="mono">${zone.id}</td><td>${zone.created_at||'—'}</td><td><a class="button" href=${href(`/providers/${params.id}/zones/${encodeProviderZone(zone.id)}`)}>管理记录</a></td></tr>`)}</tbody></table>${!zones.length&&!error?html`<div class="empty-state">没有可管理的托管区域</div>`:null}</div>
  </section>`;
}
