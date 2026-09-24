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

function App() {
  const [projects, setProjects] = useState([]);
  const [error, setError] = useState('');
  const [name, setName] = useState('');
  const [repo, setRepo] = useState('');
  const [selected, setSelected] = useState(null);
  const [deployments, setDeployments] = useState([]);
  const [secret, setSecret] = useState('');
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try { setProjects(await api.list()); setError(''); }
    catch (cause) { setError(cause.message); }
  };
  useEffect(() => { load(); }, []);
  const selectProject = async project => {
    setSelected(project);
    setSecret('');
    try { setDeployments(await api.deployments(project.id)); }
    catch (cause) { setError(cause.message); }
  };
  const create = async event => {
    event.preventDefault(); setBusy(true);
    try {
      const result = await api.create({ name, repo, webhook_branch: 'main' });
      setName(''); setRepo('');
      if (result.webhook_secret_once) setSecret(result.webhook_secret_once);
      await load();
    } catch (cause) { setError(cause.message); }
    finally { setBusy(false); }
  };
  const start = async () => {
    if (!selected) return;
    setBusy(true);
    try { await api.start(selected.id); await selectProject(selected); }
    catch (cause) { setError(cause.message); }
    finally { setBusy(false); }
  };
  const remove = async project => {
    if (!confirm(`删除项目“${project.name}”及其部署记录？`)) return;
    try { await api.remove(project.id); if (selected?.id === project.id) setSelected(null); await load(); }
    catch (cause) { setError(cause.message); }
  };

  return html`<div>
    <header><h1>项目部署</h1><p>管理 Git 仓库并查看部署状态。</p></header>
    ${error && html`<p class="error" role="alert">${error}</p>`}
    ${secret && html`<section class="notice-card"><strong>Webhook Secret（仅显示一次）</strong><pre>${secret}</pre></section>`}
    <section class="section">
      <h2>新建项目</h2>
      <form onSubmit=${create}>
        <div class="form-group"><label>项目名称</label><input class="input" required value=${name} onInput=${event => setName(event.currentTarget.value)} /></div>
        <div class="form-group"><label>Git 仓库</label><input class="input" required value=${repo} onInput=${event => setRepo(event.currentTarget.value)} placeholder="https://github.com/owner/repo.git" /></div>
        <button class="button button-primary" disabled=${busy}>创建项目</button>
      </form>
    </section>
    <section class="section">
      <h2>项目</h2>
      ${projects.length ? html`<div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>仓库</th><th>操作</th></tr></thead><tbody>
        ${projects.map(project => html`<tr key=${project.id}><td>${project.name}</td><td>${project.repo}</td><td><button class="button" onclick=${() => selectProject(project)}>部署详情</button> <button class="button button-danger" onclick=${() => remove(project)}>删除</button></td></tr>`)}
      </tbody></table></div>` : html`<p>还没有项目。</p>`}
    </section>
    ${selected && html`<section class="section"><h2>${selected.name} 的部署</h2><button class="button button-primary" disabled=${busy} onclick=${start}>立即部署</button>
      ${deployments.length ? html`<ul>${deployments.map(item => html`<li key=${item.id}>#${item.id} · ${item.status} · ${item.commit_msg || item.commit || item.triggered_by}</li>`)}</ul>` : html`<p>暂无部署记录。</p>`}
    </section>`}
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
