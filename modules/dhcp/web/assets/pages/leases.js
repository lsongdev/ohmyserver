import { html, useEffect, useState } from '@panel/shared/ui.js';

export default function Leases({ api, staticOnly = false, navigate }) {
  const [items, setItems] = useState([]); const [error, setError] = useState('');
  const empty={mac:'',ip:'',hostname:'',description:''};
  const [draft, setDraft] = useState(empty); const [editing,setEditing]=useState(null); const [busy,setBusy]=useState(false);
  const load=()=> (staticOnly ? api.listStaticLeases() : api.listLeases()).then(setItems).catch(err=>setError(err.message||'租约读取失败'));
  useEffect(() => {
    const controller = new AbortController();
    (staticOnly ? api.listStaticLeases : api.listLeases)({signal:controller.signal}).then(setItems).catch(err=>{if(err.name!=='AbortError')setError(err.message||'租约读取失败');});
    return ()=>controller.abort();
  },[api,staticOnly]);
  const remove=async item=>{setBusy(true);setError('');try{if(staticOnly)await api.deleteStaticLease(item.id);else await api.deleteLease(item.id);await load();}catch(err){setError(err.message||'删除失败');}finally{setBusy(false);}};
  const save=async event=>{event.preventDefault();setBusy(true);setError('');try{if(editing)await api.updateStaticLease(editing,draft);else await api.createStaticLease(draft);setEditing(null);setDraft(empty);await load();}catch(err){setError(err.message||'保存静态租约失败');}finally{setBusy(false);}};
  const change=(key,value)=>setDraft({...draft,[key]:value});
  return html`<section><div class="page-toolbar"><h2>${staticOnly?'静态租约':'动态租约'}</h2><div class="button-group">${staticOnly?html`<button class="button button-primary" onClick=${()=>navigate('/static-leases/new')}>添加静态租约</button>`:null}<button class="button" disabled=${busy} onClick=${load}>刷新</button></div></div>
    ${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    ${staticOnly&&editing?html`<form class="dhcp-reservation-form" onSubmit=${save}><input class="input" placeholder="MAC 地址" value=${draft.mac} onInput=${e=>change('mac',e.currentTarget.value)} required/><input class="input" placeholder="IPv4 地址" value=${draft.ip} onInput=${e=>change('ip',e.currentTarget.value)} required/><input class="input" placeholder="主机名" value=${draft.hostname} onInput=${e=>change('hostname',e.currentTarget.value)}/><input class="input" placeholder="描述" value=${draft.description} onInput=${e=>change('description',e.currentTarget.value)}/><button class="button button-primary" disabled=${busy}>保存修改</button><button class="button" type="button" onClick=${()=>{setEditing(null);setDraft(empty)}}>取消</button></form>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>IP</th><th>MAC</th><th>主机名</th>${staticOnly?html`<th>描述</th>`:html`<th>状态</th><th>到期时间</th>`}<th>操作</th></tr></thead>
      <tbody>${items.map(item=>html`<tr key=${item.id}><td class="mono">${item.ip}</td><td class="mono">${item.mac}</td><td>${item.hostname}</td>${staticOnly?html`<td>${item.description}</td>`:html`<td>${item.state}</td><td>${item.expires_at}</td>`}<td><div class="button-group">${staticOnly?html`<button class="button" disabled=${busy} onClick=${()=>{setEditing(item.id);setDraft({mac:item.mac,ip:item.ip,hostname:item.hostname||'',description:item.description||''})}}>编辑</button>`:null}<button class="button button-danger" disabled=${busy} onClick=${()=>remove(item)}>删除</button></div></td></tr>`)}</tbody>
    </table>${!items.length&&!error?html`<div class="empty-state">暂无租约</div>`:null}</div></section>`;
}

export function StaticLeases(props) { return Leases({ ...props, staticOnly: true }); }
