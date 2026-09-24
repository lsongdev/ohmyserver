import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/yeelight';
async function request(path, options = {}) {
  const response = await fetch(`${base}${path}`, { headers: { 'Content-Type': 'application/json' }, ...options });
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error?.message || `HTTP ${response.status}`);
  return result.data;
}
function App() {
  const [items, setItems] = useState([]), [found, setFound] = useState([]), [states, setStates] = useState({}), [error, setError] = useState('');
  const refresh = () => request('/devices').then(setItems).catch(e => setError(e.message));
  useEffect(() => { refresh(); }, []);
  async function add(event) {
    event.preventDefault(); const form = event.currentTarget; const data = Object.fromEntries(new FormData(form)); data.port = Number(data.port || 55443);
    try { await request('/devices', { method: 'POST', body: JSON.stringify(data) }); form.reset(); refresh(); }
    catch (e) { setError(e.message); }
  }
  async function discover() {
    try { setFound(await request('/discovery')); setError(''); }
    catch (e) { setError(e.message); }
  }
  async function addFound(device) {
    try { await request('/devices', { method: 'POST', body: JSON.stringify({ host: device.host, port: device.port, name: device.name }) }); setFound(current => current.filter(x => x.host !== device.host || x.port !== device.port)); refresh(); }
    catch (e) { setError(e.message); }
  }
  async function state(id) {
    try { const value = await request(`/devices/${id}/state`); setStates(current => ({ ...current, [id]: value })); }
    catch (e) { setError(e.message); }
  }
  async function control(id, action, value = 0, color = '') {
    try { await request(`/devices/${id}/control`, { method: 'POST', body: JSON.stringify({ action, value, color }) }); await state(id); }
    catch (e) { setError(e.message); }
  }
  async function remove(id) {
    try { await request(`/devices/${id}`, { method: 'DELETE' }); refresh(); }
    catch (e) { setError(e.message); }
  }
  return html`<section class="section"><h1>Yeelight devices</h1>
    ${error && html`<p role="alert">${error}</p>`}
    <button class="button" onClick=${discover}>Discover devices</button>
    ${found.length > 0 && html`<ul>${found.map(device => html`<li>${device.name} · ${device.host}:${device.port} <button class="button button-primary" onClick=${() => addFound(device)}>Add</button></li>`)}</ul>`}
    <form onSubmit=${add}><input class="input" name="name" placeholder="Name"><input class="input" name="host" placeholder="Host" required><input class="input" name="port" type="number" value="55443"><button class="button button-primary">Add device</button></form>
    <ul>${items.map(device => html`<li><strong>${device.name}</strong> ${device.host}:${device.port} (${device.status})
      <button class="button" onClick=${() => state(device.id)}>Read state</button>
      <button class="button" onClick=${() => control(device.id, 'toggle')}>Toggle</button>
      <button class="button" onClick=${() => control(device.id, 'brightness', 100)}>Full brightness</button>
      <button class="button button-danger" onClick=${() => remove(device.id)}>Remove</button>
      ${states[device.id] && html`<p>${states[device.id].power} · ${states[device.id].bright}% · ${states[device.id].ct}K</p>`}
    </li>`)}</ul>
  </section>`;
}
render(html`<${App} />`, document.getElementById('app'));
