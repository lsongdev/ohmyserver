import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/widgets';
async function request(path, options = {}) { const r = await fetch(`${base}${path}`, { headers: { 'Content-Type': 'application/json' }, ...options }); if (!r.ok) throw new Error(`HTTP ${r.status}`); return r.status === 204 ? null : r.json(); }
function App() {
  const [items, setItems] = useState([]), [error, setError] = useState('');
  const refresh = () => request('/widgets').then(x => setItems(x.data)).catch(e => setError(e.message));
  useEffect(() => { refresh(); }, []);
  async function create(e) { e.preventDefault(); const form = e.currentTarget; const data = Object.fromEntries(new FormData(form)); data.enabled = form.elements.enabled.checked; data.sort_order = Number(data.sort_order || 0); try { await request('/widgets', { method: 'POST', body: JSON.stringify(data) }); form.reset(); refresh(); } catch (err) { setError(err.message); } }
  async function remove(id) { try { await request(`/widgets/${id}`, { method: 'DELETE' }); refresh(); } catch (err) { setError(err.message); } }
  return html`<section class="section"><h1>Widgets</h1>${error && html`<p role="alert">${error}</p>`}<form onSubmit=${create}><input class="input" name="name" placeholder="Name" required><input class="input" name="type" placeholder="Type" required><input class="input" name="sort_order" type="number" value="0"><input class="input" name="config" placeholder="Config"><label><input class="input" name="enabled" type="checkbox" checked> Enabled</label><button class="button button-primary">Add widget</button></form><table class="table"><thead><tr><th>Name</th><th>Type</th><th>Enabled</th><th></th></tr></thead><tbody>${items.map(x => html`<tr><td>${x.name}</td><td>${x.type}</td><td>${x.enabled ? 'Yes' : 'No'}</td><td><button class="button button-danger" onClick=${() => remove(x.id)}>Delete</button></td></tr>`)}</tbody></table></section>`;
}
render(html`<${App} />`, document.getElementById('app'));
