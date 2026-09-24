import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
const api = document.querySelector('meta[name="api-base"]')?.content || '/v1/esxi';
async function request(path, options = {}) { const r = await fetch(`${api}${path}`, { headers: { 'Content-Type': 'application/json' }, ...options }); const data = await r.json().catch(() => ({})); if (!r.ok) throw new Error(data.error?.message || `HTTP ${r.status}`); return data.data; }
function App() {
 const [items,setItems]=useState([]),[error,setError]=useState('');
 const refresh=()=>request('/servers').then(setItems).catch(e=>setError(e.message)); useEffect(()=>{refresh()},[]);
 async function create(e){e.preventDefault();const f=e.currentTarget;const d=Object.fromEntries(new FormData(f));d.port=Number(d.port||443);d.insecure=f.elements.insecure.checked;try{await request('/servers',{method:'POST',body:JSON.stringify(d)});f.reset();refresh()}catch(x){setError(x.message)}}
 async function remove(id){try{await request(`/servers/${id}`,{method:'DELETE'});refresh()}catch(x){setError(x.message)}}
 return html`<section class="section"><h1>ESXi servers</h1>${error&&html`<p role="alert">${error}</p>`}<form onSubmit=${create}><input class="input" name="name" placeholder="Name" required><input class="input" name="host" placeholder="Host" required><input class="input" name="username" placeholder="Username" required><input class="input" name="password" type="password" placeholder="Password" required><input class="input" name="port" type="number" value="443"><label><input class="input" type="checkbox" name="insecure" checked> Skip TLS verification</label><button class="button button-primary">Add server</button></form><table class="table"><thead><tr><th>Name</th><th>Host</th><th>User</th><th></th></tr></thead><tbody>${items.map(x=>html`<tr><td>${x.name}</td><td>${x.host}:${x.port}</td><td>${x.username}</td><td><button class="button button-danger" onClick=${()=>remove(x.id)}>Delete</button></td></tr>`)}</tbody></table></section>`;
}
render(html`<${App}/>`,document.getElementById('app'));
