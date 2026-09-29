import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

const empty = () => ({ name:'', type:'', config:'', sort_order:0, enabled:true });
export default function WidgetList({ api, navigate }) {
  const [items,setItems]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=async(signal)=>setItems(await api.list({signal}));
  useEffect(()=>{const controller=new AbortController();load(controller.signal).catch(e=>{if(e.name!=='AbortError')setError(e.message)});return()=>controller.abort()},[api]);
  const run=async(fn)=>{setBusy(true);setError('');try{await fn();await load()}catch(e){setError(e.message||'操作失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>首页组件</h2><div class="button-group"><button class="button button-primary" onClick=${()=>navigate('/new')}>添加组件</button><button class="button" disabled=${busy} onClick=${()=>run(()=>load())}>刷新</button></div></div>
    ${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="table-wrap"><table class="table"><thead><tr><th>排序</th><th>名称</th><th>类型</th><th>配置</th><th>状态</th><th>操作</th></tr></thead><tbody>${items.map(item=>html`<tr key=${item.id}><td>${item.sort_order}</td><td>${item.name}</td><td class="mono">${item.type}</td><td class="mono widget-config-view">${item.config||'—'}</td><td>${item.enabled?'启用':'停用'}</td><td><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.remove(item.id))}>删除</button></td></tr>`)}</tbody></table>${!items.length?html`<div class="empty-state">暂无首页组件配置</div>`:null}</div>
  </section>`;
}
