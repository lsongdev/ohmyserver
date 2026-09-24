import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '';
const request = async (path, options = {}) => {
  const headers = new Headers(options.headers || {});
  if (options.body !== undefined) headers.set('Content-Type', 'application/json');
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  if (response.status === 204) return null;
  const envelope = await response.json();
  if (!response.ok) throw new Error(envelope.error?.message || `HTTP ${response.status}`);
  return envelope.data;
};
const api = createAPI({ request });
const sections = ['overview', 'network', 'dhcp', 'packages', 'firewall', 'services'];

function App() {
  const [routers, setRouters] = useState([]);
  const [selected, setSelected] = useState(null);
  const [section, setSection] = useState('overview');
  const [details, setDetails] = useState(null);
  const [form, setForm] = useState({ name: '', host: '', port: 80, username: 'root', password: '', protocol: 'http', description: '' });
  const [error, setError] = useState('');
  const refresh = async signal => {
    try { setRouters(await api.listRouters({ signal }) || []); setError(''); }
    catch (reason) { if (reason.name !== 'AbortError') setError(reason.message); }
  };
  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal);
    return () => controller.abort();
  }, []);
  useEffect(() => {
    if (!selected) { setDetails(null); return undefined; }
    const controller = new AbortController();
    api.readSection(selected.id, section, { signal: controller.signal }).then(setDetails)
      .catch(reason => { if (reason.name !== 'AbortError') setError(reason.message); });
    return () => controller.abort();
  }, [selected, section]);
  const create = async event => {
    event.preventDefault();
    try {
      const item = await api.createRouter({ ...form, port: Number(form.port) });
      setForm(current => ({ ...current, name: '', host: '', password: '', description: '' }));
      await refresh(); setSelected(item);
    } catch (reason) { setError(reason.message); }
  };
  const test = async router => {
    try { await api.testRouter(router.id); setError(`${router.name} 连接成功`); }
    catch (reason) { setError(reason.message); }
  };
  const remove = async router => {
    try { await api.deleteRouter(router.id); if (selected?.id === router.id) setSelected(null); await refresh(); }
    catch (reason) { setError(reason.message); }
  };
  return html`<div class="openwrt-standalone-layout">
    <section class="section">
      <h2>路由器</h2>
      ${routers.map(router => html`<article class="openwrt-router-row" key=${router.id}>
        <button class="button" onClick=${() => setSelected(router)}>${router.name} · ${router.host}</button>
        <button class="button" onClick=${() => test(router)}>连接测试</button>
        <button class="button button-danger" onClick=${() => remove(router)}>删除</button>
      </article>`)}
      <form class="openwrt-router-form" onSubmit=${create}>
        <input class="input" value=${form.name} placeholder="名称" required onInput=${e => setForm({...form, name:e.currentTarget.value})}>
        <input class="input" value=${form.host} placeholder="地址" required onInput=${e => setForm({...form, host:e.currentTarget.value})}>
        <input class="input" type="number" value=${form.port} min="1" max="65535" required onInput=${e => setForm({...form, port:e.currentTarget.value})}>
        <input class="input" value=${form.username} placeholder="用户名" required onInput=${e => setForm({...form, username:e.currentTarget.value})}>
        <input class="input" type="password" value=${form.password} placeholder="密码" required onInput=${e => setForm({...form, password:e.currentTarget.value})}>
        <button class="button button-primary" type="submit">添加路由器</button>
      </form>
      ${error ? html`<p class="openwrt-error">${error}</p>` : null}
    </section>
    ${selected ? html`<section class="section">
      <h2>${selected.name}</h2>
      <nav class="openwrt-sections">${sections.map(item => html`<button class="button" onClick=${() => setSection(item)}>${item}</button>`)}</nav>
      <pre class="openwrt-data">${JSON.stringify(details, null, 2) || '加载中…'}</pre>
      <button class="button button-danger" onClick=${() => api.action(selected.id,{action:'reboot'}).catch(reason=>setError(reason.message))}>重启设备</button>
    </section>` : null}
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
