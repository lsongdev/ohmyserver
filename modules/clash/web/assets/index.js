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
const api = createAPI({ request, apiURL: path => `${apiBase}${path}` });

function navigate(path) {
  location.hash = path || '/';
}

function ServerForm({ id, onSaved }) {
  const editing = Boolean(id);
  const [form, setForm] = useState({ name: '', api_url: '', secret: '', description: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!editing) return undefined;
    const controller = new AbortController();
    api.get(id, { signal: controller.signal }).then(value => setForm({ ...value, secret: '' }))
      .catch(reason => { if (reason.name !== 'AbortError') setError(reason.message); });
    return () => controller.abort();
  }, [id, editing]);
  const update = (key, nextValue) => setForm(current => ({ ...current, [key]: nextValue }));
  const submit = async event => {
    event.preventDefault();
    setBusy(true);
    setError('');
    const input = { name: form.name, api_url: form.api_url, description: form.description };
    if (form.secret) input.secret = form.secret;
    try {
      if (editing) await api.update(id, input);
      else await api.create({ ...input, secret: form.secret });
      onSaved();
    } catch (reason) {
      setError(reason.message);
    } finally {
      setBusy(false);
    }
  };
  return html`<div>
    <button class="button" onclick=${() => navigate('/')}>← 返回实例</button>
    <h2>${editing ? '编辑 Clash' : '添加 Clash'}</h2>
    ${error ? html`<p class="clash-standalone-notice">${error}</p>` : null}
    <form class="card" onSubmit=${submit}>
      <div class="form-group"><label>名称</label><input class="input" required value=${form.name} onInput=${e => update('name', e.currentTarget.value)} /></div>
      <div class="form-group"><label>API 地址</label><input class="input" type="url" required value=${form.api_url} onInput=${e => update('api_url', e.currentTarget.value)} /></div>
      <div class="form-group"><label>密钥${editing ? '（留空保持不变）' : ''}</label><input class="input" type="password" value=${form.secret} onInput=${e => update('secret', e.currentTarget.value)} autocomplete="new-password" /></div>
      <div class="form-group"><label>描述</label><textarea class="input" rows="3" value=${form.description} onInput=${e => update('description', e.currentTarget.value)}></textarea></div>
      <div class="form-actions"><button class="button button-primary" disabled=${busy}>保存</button><button class="button" type="button" onclick=${() => navigate('/')}>取消</button></div>
    </form>
  </div>`;
}

function App() {
  const [route, setRoute] = useState(location.hash.replace(/^#/, '') || '/');
  const [items, setItems] = useState([]);
  const [message, setMessage] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const onHashChange = () => setRoute(location.hash.replace(/^#/, '') || '/');
    addEventListener('hashchange', onHashChange);
    return () => removeEventListener('hashchange', onHashChange);
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    api.list({ signal: controller.signal }).then(setItems)
      .catch(reason => { if (reason.name !== 'AbortError') setMessage(reason.message); });
    return () => controller.abort();
  }, [refresh]);

  const edit = route.match(/^\/(\d+)\/edit$/);
  if (route === '/new' || edit) {
    return html`<${ServerForm} id=${edit?.[1]} onSaved=${() => { navigate('/'); setRefresh(value => value + 1); }} />`;
  }
  const test = async id => {
    setMessage('');
    try { await api.test(id); setMessage('连接成功'); }
    catch (reason) { setMessage(reason.message); }
  };
  const remove = async id => {
    if (!confirm('确定删除此 Clash 配置？')) return;
    try { await api.remove(id); setRefresh(value => value + 1); }
    catch (reason) { setMessage(reason.message); }
  };
  return html`<div>
    <div class="clash-standalone-toolbar"><h2>代理实例</h2><button class="button button-primary" onclick=${() => navigate('/new')}>添加 Clash</button></div>
    ${message ? html`<p class="clash-standalone-notice">${message}</p>` : null}
    <div class="table-container"><table class="table"><thead><tr><th>名称</th><th>API 地址</th><th>密钥</th><th>状态</th><th>操作</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.name}</td><td>${item.api_url}</td><td>${item.has_secret ? '已配置' : '未配置'}</td><td>${item.status}</td><td><div class="clash-standalone-actions"><button class="button" onclick=${() => test(item.id)}>测试</button><button class="button" onclick=${() => navigate(`/${item.id}/edit`)}>编辑</button><button class="button button-danger" onclick=${() => remove(item.id)}>删除</button></div></td></tr>`)}</tbody>
    </table>${items.length ? null : html`<div class="clash-standalone-empty">暂无 Clash 实例</div>`}</div>
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
