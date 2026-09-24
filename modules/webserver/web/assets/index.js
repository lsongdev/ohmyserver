import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/webserver';
const request = async (path, options = {}) => {
  const response = await fetch(`${base}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
    body: options.body && typeof options.body !== 'string' ? JSON.stringify(options.body) : options.body,
  });
  if (response.status === 204) return null;
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error?.message || `HTTP ${response.status}`);
  return result.data;
};
const api = createAPI({ request });
const routes = [
  { path: '/listeners', load: () => import('./pages/listeners.js') },
  { path: '/listeners/new', load: () => import('./pages/listener-new.js') },
  { path: '/policies', load: () => import('./pages/policies.js') },
  { path: '/policies/new', load: () => import('./pages/policy-new.js') },
  { path: '/hosts', load: () => import('./pages/hosts.js') },
  { path: '/hosts/new', load: () => import('./pages/host-new.js') },
  { path: '/hosts/:id/routes/new', load: () => import('./pages/route-new.js') },
];
function currentRoute() {
  const path = location.hash.slice(1) || '/listeners';
  for (const route of routes) {
    const names = [];
    const pattern = route.path.replace(/:[^/]+/g, name => { names.push(name.slice(1)); return '([^/]+)'; });
    const match = path.match(new RegExp(`^${pattern}/?$`));
    if (match) return { ...route, path, params: Object.fromEntries(names.map((name, i) => [name, decodeURIComponent(match[i + 1])])) };
  }
  return { path: '/listeners', params: {}, load: () => import('./pages/listeners.js') };
}
function App() {
  const [route, setRoute] = useState(currentRoute);
  const [Page, setPage] = useState(null);
  const [error, setError] = useState('');
  useEffect(() => {
    const changed = () => { setRoute(currentRoute()); setPage(null); };
    addEventListener('hashchange', changed);
    return () => removeEventListener('hashchange', changed);
  }, []);
  useEffect(() => {
    let active = true;
    route.load().then(module => { if (active) { setPage(() => module.default); setError(''); } }).catch(err => { if (active) setError(err.message); });
    return () => { active = false; };
  }, [route.path]);
  const navigate = path => { location.hash = path.replace(/^\/webserver/, '') || '/listeners'; };
  return html`<div class="webserver-app">
    <header><h1>Web Server</h1><nav class="webserver-nav"><a href="#/listeners">Listeners</a><a href="#/hosts">Virtual Hosts</a><a href="#/policies">Policies</a></nav></header>
    <main>${error ? html`<p class="notice-card error" role="alert">${error}</p>` : Page ? html`<${Page} api=${api} params=${route.params} navigate=${navigate}/>` : html`<p>Loading…</p>`}</main>
  </div>`;
}
render(html`<${App}/>`, document.getElementById('app'));
