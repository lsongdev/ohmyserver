import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';

const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/docker';
const endpoints = { containers: '/containers', images: '/images', volumes: '/volumes', networks: '/networks', compose: '/compose', apps: '/apps' };

async function request(path, options = {}) {
  const response = await fetch(`${base}${path}`, { ...options, headers: { 'Content-Type': 'application/json', ...options.headers } });
  const body = response.status === 204 ? {} : await response.json();
  if (!response.ok) throw new Error(body.error?.message || `HTTP ${response.status}`);
  return body.data;
}

function App() {
  const [view, setView] = useState('containers');
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [image, setImage] = useState('');
  const [name, setName] = useState('');
  const [command, setCommand] = useState('');
  const [composeName, setComposeName] = useState('');
  const [composeText, setComposeText] = useState('');
  async function load(next = view) {
    try { const data = await request(endpoints[next]); setItems(Array.isArray(data) ? data : data?.Volumes || data?.Networks || []); setError(''); }
    catch (err) { setError(err.message || '读取 Docker 数据失败'); setItems([]); }
  }
  useEffect(() => { load(); }, [view]);
  async function act(path, method = 'POST', body) {
    setBusy(true); setError('');
    try { await request(path, { method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }); await load(); }
    catch (err) { setError(err.message || 'Docker 操作失败'); } finally { setBusy(false); }
  }
  async function pull() {
    setBusy(true); setError('');
    try { const response = await fetch(`${base}/images/pull`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: image }) }); const text = await response.text(); if (!response.ok) throw new Error(text); await load('images'); }
    catch (err) { setError(err.message || '拉取镜像失败'); } finally { setBusy(false); }
  }
  const rowName = item => item.Names?.join(', ') || item.Name || item.name || item.Id || item.ID || item.id || JSON.stringify(item);
  return html`<section class="section docker-standalone">
    <h1>Docker</h1>
    ${error && html`<p role="alert">${error}</p>`}
    <nav class="form-actions">${Object.keys(endpoints).map(key => html`<button class="button" disabled=${busy} onClick=${() => setView(key)}>${key}</button>`)}<button class="button" disabled=${busy} onClick=${() => load()}>刷新</button></nav>
    ${view === 'containers' && html`<form class="form-actions" onSubmit=${event => { event.preventDefault(); act('/containers', 'POST', { name, image, command }); }}><input class="input" placeholder="容器名称" value=${name} onInput=${e => setName(e.currentTarget.value)} required /><input class="input" placeholder="镜像" value=${image} onInput=${e => setImage(e.currentTarget.value)} required /><input class="input" placeholder="启动命令（可选）" value=${command} onInput=${e => setCommand(e.currentTarget.value)} /><button class="button button-primary" disabled=${busy}>创建容器</button></form>`}
    ${view === 'images' && html`<form class="form-actions" onSubmit=${event => { event.preventDefault(); pull(); }}><input class="input" placeholder="镜像名，例如 alpine:latest" value=${image} onInput=${e => setImage(e.currentTarget.value)} required /><button class="button button-primary" disabled=${busy}>拉取镜像</button></form>`}
    ${view === 'compose' && html`<form onSubmit=${event => { event.preventDefault(); act('/compose', 'POST', { name: composeName, source: 'file', compose: composeText }); }}><input class="input" placeholder="项目名称" value=${composeName} onInput=${e => setComposeName(e.currentTarget.value)} required /><textarea class="input" rows="10" placeholder="Compose YAML" value=${composeText} onInput=${e => setComposeText(e.currentTarget.value)} required></textarea><button class="button button-primary" disabled=${busy}>创建并启动</button></form>`}
    ${view === 'apps' && html`<div class="form-actions"><button class="button" disabled=${busy} onClick=${() => act('/apps/sync','POST')}>同步应用清单</button></div>`}
    <table class="table"><thead><tr><th>资源</th><th>详情</th><th>操作</th></tr></thead><tbody>${items.map(item => {
      const id = item.ID || item.Id || item.Name || item.name;
      const actions = view === 'containers' ? ['start','stop','restart','delete'] : view === 'compose' ? ['up','down','restart','pull'] : ['delete'];
      return html`<tr><td>${rowName(item)}</td><td><pre>${JSON.stringify(item, null, 2)}</pre></td><td>${actions.map(action => html`<button class="button" disabled=${busy} onClick=${() => act(view === 'containers' ? `/containers/${encodeURIComponent(id)}/${action}` : view === 'compose' ? `/compose/${encodeURIComponent(id)}/${action}` : `/${view}/${encodeURIComponent(id)}`, action === 'delete' ? 'DELETE' : 'POST')}>${action}</button>`)}</td></tr>`;
    })}</tbody></table>
    ${items.length === 0 && html`<p>没有资源，或当前列表为空。</p>`}
  </section>`;
}

render(html`<${App} />`, document.getElementById('app'));
