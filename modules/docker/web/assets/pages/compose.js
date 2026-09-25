import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Compose({ api, navigate }) {
  const [items, setItems] = useState([]); const [error, setError] = useState('');
  useEffect(() => { const controller = new AbortController(); api.listComposeProjects({ signal: controller.signal }).then(setItems).catch(err => { if (err.name !== 'AbortError') setError(err.message || 'Compose 项目读取失败'); }); return () => controller.abort(); }, [api]);
  return html`<div><div class="page-toolbar"><h2>Compose 项目</h2><div class="button-group"><button class="button button-primary" onclick=${() => navigate('/compose/new')}>新建项目</button><button class="button" onclick=${() => location.reload()}>刷新</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>项目</th><th>状态</th><th>配置文件</th><th>更新时间</th></tr></thead><tbody>${items.map(item => html`<tr key=${item.name}><td><a href=${`#/docker/compose/${encodeURIComponent(item.name)}`}>${item.name}</a></td><td>${item.status}</td><td class="mono">${item.config_files}</td><td>${item.updated_at ? new Date(item.updated_at).toLocaleString() : '—'}</td></tr>`)}</tbody></table>${!items.length && !error ? html`<div class="empty-state">未发现 Compose 项目</div>` : null}</div>
  </div>`;
}
