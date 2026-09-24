import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';

const api = document.querySelector('meta[name="api-base"]')?.content || '/v1/uptime';
async function request(path, options) {
  const response = await fetch(`${api}${path}`, { headers: { 'Content-Type': 'application/json' }, ...options });
  if (!response.ok) throw new Error((await response.json()).error?.message || `HTTP ${response.status}`);
  return response.status === 204 ? null : response.json();
}
function App() {
  const [monitors, setMonitors] = useState([]);
  const [error, setError] = useState('');
  const refresh = () => request('/monitors').then(result => setMonitors(result.data)).catch(err => setError(err.message));
  useEffect(() => { refresh(); }, []);
  async function create(event) {
    event.preventDefault();
    const data = Object.fromEntries(new FormData(event.currentTarget));
    data.enabled = true; data.interval = 60; data.timeout = 10;
    try { await request('/monitors', { method: 'POST', body: JSON.stringify(data) }); event.currentTarget.reset(); refresh(); }
    catch (err) { setError(err.message); }
  }
  async function action(id, name) { try { await request(`/monitors/${id}/actions/${name}`, { method: 'POST' }); refresh(); } catch (err) { setError(err.message); } }
  async function remove(id) { try { await request(`/monitors/${id}`, { method: 'DELETE' }); refresh(); } catch (err) { setError(err.message); } }
  return html`<section class="section"><h1>Uptime monitors</h1>
    ${error && html`<p role="alert">${error}</p>`}
    <form onSubmit=${create} class="form-group">
      <input class="input" name="name" placeholder="Name" required>
      <select class="input" name="type"><option value="http">HTTP</option><option value="tcp">TCP</option><option value="ping">Ping</option></select>
      <input class="input" name="target" placeholder="Target" required>
      <button class="button button-primary" type="submit">Add monitor</button>
    </form>
    <table class="table"><thead><tr><th>Name</th><th>Type</th><th>Target</th><th>Status</th><th>Actions</th></tr></thead><tbody>
      ${monitors.map(item => html`<tr><td>${item.name}</td><td>${item.type}</td><td>${item.target}</td><td>${item.last_check?.status || 'unknown'}</td><td><button class="button" onClick=${() => action(item.id, 'check')}>Check</button> <button class="button" onClick=${() => action(item.id, 'toggle')}>Toggle</button> <button class="button button-danger" onClick=${() => remove(item.id)}>Delete</button></td></tr>`)}
    </tbody></table>
  </section>`;
}
render(html`<${App} />`, document.getElementById('app'));
