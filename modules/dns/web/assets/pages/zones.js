import { html } from '@module/ui';
import { useEffect,useState } from 'preact/hooks';

export default function Zones({api,href,navigate}){
  const [items,setItems]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>api.listZones().then(setItems).catch(err=>setError(err.message||'区域读取失败'));
  useEffect(()=>{const controller=new AbortController();api.listZones({signal:controller.signal}).then(setItems).catch(err=>{if(err.name!=='AbortError')setError(err.message||'区域读取失败')});return()=>controller.abort()},[api]);
  const remove=async zone=>{if(!confirm(`删除区域 ${zone.name} 及其所有记录？`))return;setBusy(true);setError('');try{await api.deleteZone(zone.id);await load()}catch(err){setError(err.message||'删除失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>本地区域</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/zones/new')}>创建区域</button><button class="button" disabled=${busy} onClick=${load}>刷新</button></div></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>区域</th><th>说明</th><th>状态</th><th>修改时间</th><th>操作</th></tr></thead><tbody>${items.map(zone=>html`<tr key=${zone.id}><td><a href=${href(`/zones/${zone.id}`)}>${zone.name}</a></td><td>${zone.description}</td><td>${zone.enabled?'启用':'停用'}</td><td>${zone.updated_at}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>remove(zone)}>删除</button></td></tr>`)}</tbody></table>${!items.length&&!error?html`<div class="empty-state">暂无本地区域</div>`:null}</div>
  </section>`;
}
