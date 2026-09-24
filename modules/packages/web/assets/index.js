import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '/v1/packages';
const api = createAPI({ request: async (path, options = {}) => {
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    body: options.body == null ? undefined : JSON.stringify(options.body),
    headers: options.body == null ? options.headers : { 'Content-Type': 'application/json', ...options.headers },
  });
  const body = response.status === 204 ? null : await response.json();
  if (!response.ok) throw new Error(body?.error?.message || `HTTP ${response.status}`);
  return body?.data ?? body;
} });

function App() {
  const [view, setView] = useState('installed');
  const [query, setQuery] = useState('');
  const [catalog, setCatalog] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = async (nextView = view, nextQuery = query) => {
    setError('');
    try { setCatalog(await api.list(nextQuery ? 'search' : nextView, nextQuery)); }
    catch (reason) { setError(reason.message || '软件包读取失败'); }
  };
  useEffect(() => { load('installed', ''); }, []);
  const act = async (operation, name = '') => {
    setBusy(true); setError('');
    try {
      await api[operation](name);
      await load(view, query);
    } catch (reason) { setError(reason.message || '软件包操作失败'); }
    finally { setBusy(false); }
  };
  const search = event => { event.preventDefault(); load(view, query.trim()); };
  const caps = catalog?.capabilities || {};
  return html`<section class="section">
    <div class="page-toolbar"><div><h1>软件包</h1>${catalog && html`<p>${catalog.manager} · ${catalog.executable} · ${catalog.mutation_mode}</p>`}</div>
      <div class="button-group"><button class="button" disabled=${busy || !caps.refresh} onClick=${() => act('refresh')}>刷新软件源</button><button class="button button-primary" disabled=${busy || !caps.safe_upgrade_all} onClick=${() => act('upgrade')}>全部升级</button></div>
    </div>
    <form class="form-actions" onSubmit=${search}><select class="input" value=${view} onChange=${event => { setView(event.currentTarget.value); setQuery(''); load(event.currentTarget.value, ''); }}><option value="installed">已安装</option><option value="upgradable">可升级</option><option value="search">搜索结果</option></select><input class="input" aria-label="搜索软件包" value=${query} onInput=${event => setQuery(event.currentTarget.value)} placeholder="搜索软件包"/><button class="button button-primary" disabled=${!caps.search}>搜索</button></form>
    ${error && html`<p role="alert">${error}</p>`}
    ${catalog && html`<table class="table"><thead><tr><th>名称</th><th>版本</th><th>可用版本</th><th>描述</th><th>操作</th></tr></thead><tbody>${catalog.packages.map(item => html`<tr key=${item.name}><td>${item.name}</td><td>${item.version || '—'}</td><td>${item.latest_version || '—'}</td><td>${item.description || '—'}</td><td>${view === 'upgradable' ? html`<button class="button button-primary" disabled=${busy} onClick=${() => act('upgrade', item.name)}>升级</button>` : view === 'search' && !item.installed ? html`<button class="button button-primary" disabled=${busy} onClick=${() => act('install', item.name)}>安装</button>` : html`<button class="button button-danger" disabled=${busy} onClick=${() => confirm(`移除 ${item.name}？`) && act('remove', item.name)}>移除</button>`}</td></tr>`)}</tbody></table>`}
  </section>`;
}

render(html`<${App} />`, document.getElementById('app'));
