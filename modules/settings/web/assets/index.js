import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/settings';
async function request(path, options = {}) { const r = await fetch(`${base}${path}`, { headers: { 'Content-Type': 'application/json' }, ...options }); const d = await r.json().catch(() => ({})); if (!r.ok) throw new Error(d.error?.message || `HTTP ${r.status}`); return d.data; }
function App() {
 const [items,setItems]=useState([]),[error,setError]=useState('');
 const refresh=()=>request('/').then(setItems).catch(e=>setError(e.message)); useEffect(()=>{refresh()},[]);
 async function create(e){e.preventDefault();const f=e.currentTarget;try{await request('/',{method:'POST',body:JSON.stringify(Object.fromEntries(new FormData(f)))});f.reset();refresh()}catch(x){setError(x.message)}}
 async function save(item,value){try{await request(`/${item.id}`,{method:'PUT',body:JSON.stringify({value})});refresh()}catch(x){setError(x.message)}}
 async function remove(id){try{await request(`/${id}`,{method:'DELETE'});refresh()}catch(x){setError(x.message)}}
 return html`<section class="section"><h1>Settings</h1>${error&&html`<p role="alert">${error}</p>`}<form onSubmit=${create}><input class="input" name="name" placeholder="Name" required><select class="input" name="type"><option>string</option><option>number</option><option>boolean</option><option>json</option></select><input class="input" name="value" placeholder="Value" required><input class="input" name="description" placeholder="Description"><button class="button button-primary">Add</button></form><table class="table"><thead><tr><th>Name</th><th>Type</th><th>Value</th><th>Actions</th></tr></thead><tbody>${items.map(x=>html`<tr><td>${x.name}</td><td>${x.type}</td><td><input class="input" value=${x.value} onChange=${e=>save(x,e.currentTarget.value)}></td><td><button class="button button-danger" onClick=${()=>remove(x.id)}>Delete</button></td></tr>`)}</tbody></table></section>`;
}
render(html`<${App}/>`,document.getElementById('app'));
