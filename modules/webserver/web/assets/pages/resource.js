import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Resource({api,kind,title,columns}) {
  const [items,setItems]=useState([]),[error,setError]=useState('');
  useEffect(()=>{const controller=new AbortController();api[kind]({signal:controller.signal}).then(setItems).catch(e=>{if(e.name!=='AbortError')setError(e.message)});return()=>controller.abort()},[api,kind]);
  return html`<section><div class="page-toolbar"><h2>${title}</h2><button class="button" onClick=${()=>api[kind]().then(setItems).catch(e=>setError(e.message))}>刷新</button></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}<div class="table-wrap"><table class="table"><thead><tr>${columns.map(col=>html`<th>${col.label}</th>`)}</tr></thead><tbody>${items.map(item=>html`<tr key=${item.id}>${columns.map(col=>html`<td>${col.value(item)}</td>`)}</tr>`)}</tbody></table>${!items.length?html`<div class="empty-state">暂无${title}</div>`:null}</div></section>`;
}
