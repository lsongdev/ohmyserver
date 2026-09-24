import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';

const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/system';

async function request(path, options = {}) {
  const response = await fetch(`${base}${path}`, options);
  const body = await response.json();
  if (!response.ok) throw new Error(body.error?.message || `HTTP ${response.status}`);
  return body.data;
}

function App() {
  const [info, setInfo] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  async function load(check = false) {
    setBusy(true);
    setError('');
    try { setInfo(await request(`/info${check ? '?check=1' : ''}`)); }
    catch (err) { setError(err.message || '读取系统信息失败'); }
    finally { setBusy(false); }
  }
  useEffect(() => { load(); }, []);
  const rows = info ? [
    ['版本', info.program_version], ['提交', info.program_commit], ['构建时间', info.build_time],
    ['运行环境', `${info.os} / ${info.arch}`], ['Go 版本', info.go_version], ['CPU 核心', info.num_cpu],
    ['监听地址', info.listen], ['数据库', info.database], ['运行时间', info.uptime],
  ] : [];
  return html`<section class="system-about">
    <h1>系统信息</h1>
    ${error && html`<p role="alert">${error}</p>`}
    ${!info && !error && html`<p role="status">正在读取系统信息…</p>`}
    ${info && html`<section class="card"><h2>${info.program_name}</h2><p>${info.program_desc}</p>
      <dl class="about-grid">${rows.map(([label, value]) => html`<div><dt>${label}</dt><dd>${value || '—'}</dd></div>`)}</dl>
      <div class="form-actions"><button class="button" disabled=${busy} onClick=${() => load(true)}>${busy ? '检查中…' : '检查更新'}</button></div>
      ${info.update?.checked && html`<p>${info.update.message}</p>`}
    </section>
    <section class="card"><h2>开源组件</h2><ul>${info.credits.map(item => html`<li><a href=${item.url} target="_blank" rel="noreferrer">${item.name}</a> · ${item.license}</li>`)}</ul></section>`}
  </section>`;
}

render(html`<${App} />`, document.getElementById('app'));
