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

function App() {
  const [status, setStatus] = useState(null);
  const [zones, setZones] = useState([]);
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const refresh = async signal => {
    try {
      const [nextStatus, nextZones] = await Promise.all([api.getStatus({ signal }), api.listZones({ signal })]);
      setStatus(nextStatus);
      setZones(nextZones || []);
      setError('');
    } catch (reason) {
      if (reason.name !== 'AbortError') setError(reason.message);
    }
  };
  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal);
    return () => controller.abort();
  }, []);
  const act = async action => {
    try { await api.action(action); await refresh(); }
    catch (reason) { setError(reason.message); }
  };
  const addZone = async event => {
    event.preventDefault();
    try { await api.createZone({ name }); setName(''); await refresh(); }
    catch (reason) { setError(reason.message); }
  };
  const runtime = status?.runtime || {};
  return html`<div class="dns-standalone-layout">
    <section class="section">
      <h2>运行状态</h2>
      <p>状态：${runtime.running ? '运行中' : '已停止'}</p>
      ${runtime.udp_addr ? html`<p>UDP：${runtime.udp_addr}</p>` : null}
      ${runtime.tcp_addr ? html`<p>TCP：${runtime.tcp_addr}</p>` : null}
      ${runtime.last_error ? html`<p class="dns-error">${runtime.last_error}</p>` : null}
      <div class="dns-standalone-actions">
        <button class="button button-primary" onClick=${() => act('start')}>启动</button>
        <button class="button" onClick=${() => act('reload')}>重载</button>
        <button class="button button-danger" onClick=${() => act('stop')}>停止</button>
      </div>
    </section>
    <section class="section">
      <h2>区域</h2>
      <form class="dns-standalone-form" onSubmit=${addZone}>
        <input class="input" value=${name} onInput=${event => setName(event.currentTarget.value)} placeholder="example.test" required>
        <button class="button button-primary" type="submit">添加区域</button>
      </form>
      ${error ? html`<p class="dns-error">${error}</p>` : null}
      <ul>${zones.map(zone => html`<li key=${zone.id}>${zone.name} ${zone.enabled ? '' : '(已停用)'}</li>`)}</ul>
    </section>
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
