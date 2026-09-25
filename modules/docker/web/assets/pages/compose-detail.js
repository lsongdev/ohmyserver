import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function ComposeDetail({ api, params, navigate }) {
  const [item, setItem] = useState(null); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const load = () => api.getComposeProject(params.name).then(setItem).catch(err => setError(err.message || 'Compose 项目读取失败'));
  useEffect(() => { const controller = new AbortController(); api.getComposeProject(params.name, { signal: controller.signal }).then(setItem).catch(err => { if (err.name !== 'AbortError') setError(err.message || 'Compose 项目读取失败'); }); return () => controller.abort(); }, [api, params.name]);
  const action = async verb => { setBusy(true); setError(''); try { await api.composeAction(params.name, verb); if (verb === 'down') { navigate('/compose'); return; } await load(); } catch (err) { setError(err.message || 'Compose 操作失败'); } finally { setBusy(false); } };
  const project = item?.project;
  return html`<div><button class="button button-link" onclick=${() => navigate('/compose')}>← Compose 项目</button>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    ${project ? html`<section class="section"><div class="page-toolbar"><h2>${project.name}</h2><div class="button-group"><button class="button button-primary" disabled=${busy} onclick=${() => action('up')}>启动</button><button class="button" disabled=${busy} onclick=${() => action('restart')}>重启</button><button class="button button-danger" disabled=${busy} onclick=${() => action('down')}>停止</button><button class="button" disabled=${busy} onclick=${() => action('pull')}>拉取镜像</button></div></div><dl><dt>状态</dt><dd>${project.status}</dd><dt>配置文件</dt><dd class="mono">${project.config_files}</dd></dl>
      <h3>容器</h3><div class="data-table-wrap"><table class="data-table"><thead><tr><th>服务</th><th>容器</th><th>镜像</th><th>状态</th><th>端口</th></tr></thead><tbody>${(item.containers || []).map((container, index) => html`<tr key=${container.ID || index}><td>${container.Service || '—'}</td><td>${container.Name || container.ID || '—'}</td><td>${container.Image || '—'}</td><td>${container.Status || container.State || '—'}</td><td>${container.Ports || '—'}</td></tr>`)}</tbody></table>${!item.containers?.length ? html`<div class="empty-state">没有容器</div>` : null}</div></section>` : !error ? html`<div class="loading-state">正在加载项目…</div>` : null}
  </div>`;
}
