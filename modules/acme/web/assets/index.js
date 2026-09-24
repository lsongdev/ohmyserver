import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '/v1/acme';
const api = createAPI({
  apiURL: path => `${apiBase}${path}`,
  request: async (path, options = {}) => {
    const response = await fetch(`${apiBase}${path}`, {
      ...options,
      body: options.body == null ? undefined : JSON.stringify(options.body),
      headers: options.body ? { 'Content-Type': 'application/json', ...options.headers } : options.headers,
    });
    const body = response.status === 204 ? null : await response.json();
    if (!response.ok) throw new Error(body?.error?.message || `HTTP ${response.status}`);
    return body?.data ?? body;
  },
});

const sections = [
  { id: 'certificates', title: '证书', load: () => api.listCertificates() },
  { id: 'issuances', title: '签发记录', load: () => api.listIssuances() },
  { id: 'orders', title: '订单', load: () => api.listOrders() },
  { id: 'accounts', title: '账户', load: () => api.listAccounts() },
  { id: 'providers', title: '服务商', load: () => api.listProviders() },
];

function App() {
  const [section, setSection] = useState(location.hash.slice(1) || 'certificates');
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const [result, setResult] = useState('');
  const [busy, setBusy] = useState(false);
  const current = sections.find(item => item.id === section) || sections[0];

  useEffect(() => {
    const onHashChange = () => setSection(location.hash.slice(1) || 'certificates');
    addEventListener('hashchange', onHashChange);
    return () => removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    let active = true;
    setError('');
    current.load().then(value => { if (active) setItems(Array.isArray(value) ? value : []); })
      .catch(reason => { if (active) setError(reason.message); });
    return () => { active = false; };
  }, [section]);

  const perform = async (event, action) => {
    event.preventDefault();
    const element = event.currentTarget;
    const form = new FormData(element);
    setBusy(true); setError(''); setResult('');
    try {
      const value = await action(form);
      setResult(value == null ? '操作完成' : JSON.stringify(value, null, 2));
      element.reset();
      const latest = await current.load();
      setItems(Array.isArray(latest) ? latest : []);
    } catch (reason) { setError(reason.message); }
    finally { setBusy(false); }
  };

  const field = (name, label, placeholder = '') => html`
    <label>${label}<input class="input" name=${name} placeholder=${placeholder} required /></label>`;
  const columns = current.id === 'certificates'
    ? ['id', 'domains', 'status', 'expires_at']
    : current.id === 'issuances' ? ['id', 'domains', 'status', 'step', 'error']
      : current.id === 'orders' ? ['id', 'account_id', 'domains', 'order_url']
        : current.id === 'accounts' ? ['id', 'provider_name', 'email', 'status', 'account_url']
          : ['id', 'name', 'directory_url', 'status'];

  return html`<>
    <nav class="acme-nav">${sections.map(item => html`
      <a href=${`#${item.id}`} aria-current=${section === item.id ? 'page' : null}>${item.title}</a>`)}</nav>
    <h1>${current.title}</h1>
    ${error && html`<p class="acme-message acme-error" role="alert">${error}</p>`}
    ${result && html`<pre class="acme-message acme-result">${result}</pre>`}
    <div class="acme-grid">
      <section class="acme-card"><h2>${current.title}</h2>
        ${items.length ? html`<div class="table-container"><table class="table"><thead><tr>${columns.map(key => html`<th>${key}</th>`)}</tr></thead>
          <tbody>${items.map(item => html`<tr>${columns.map(key => html`<td>${Array.isArray(item[key]) ? item[key].join(', ') : (item[key] ?? '')}</td>`)}</tr>`)}</tbody></table></div>` : html`<p>暂无${current.title}</p>`}
      </section>
      <section class="acme-card"><h2>操作</h2>
        ${section === 'providers' && html`<form class="acme-form" onSubmit=${event => perform(event, form => api.createProvider({ name: form.get('name'), directory_url: form.get('directory_url') }))}>
          ${field('name', '名称')}${field('directory_url', 'ACME Directory URL', 'https://example.test/acme/directory')}
          <button class="button" disabled=${busy}>添加服务商</button></form>`}
        ${section === 'accounts' && html`<form class="acme-form" onSubmit=${event => perform(event, form => api.registerAccount(form.get('provider_id'), { email: form.get('email'), agreed_terms: form.get('agreed_terms') === 'on' }))}>
          ${field('provider_id', '服务商 ID')}${field('email', '邮箱', 'admin@example.com')}
          <label><input class="input" type="checkbox" name="agreed_terms" />同意服务条款</label><button class="button" disabled=${busy}>注册账户</button></form>`}
        ${section === 'certificates' && html`<form class="acme-form" onSubmit=${event => perform(event, form => api.generateCSR({ domains: form.get('domains') }))}>
          ${field('domains', '域名（逗号分隔）', 'example.com, www.example.com')}<button class="button" disabled=${busy}>生成 CSR</button></form>`}
        ${section === 'issuances' && html`<form class="acme-form" onSubmit=${event => perform(event, form => api.startIssuance({ account_id: Number(form.get('account_id')), domains: form.get('domains'), challenge_type: form.get('challenge_type'), dns_target: form.get('dns_target') }))}>
          ${field('account_id', '账户 ID')}${field('domains', '域名（逗号分隔）')}
          <label>验证方式<select class="input" name="challenge_type"><option value="http-01">HTTP-01</option><option value="dns-01">DNS-01</option></select></label>
          <label>DNS 目标（DNS-01）<input class="input" name="dns_target" /></label><button class="button" disabled=${busy}>开始签发</button></form>`}
      </section>
    </div>
  </>`;
}

render(html`<${App} />`, document.getElementById('app'));
