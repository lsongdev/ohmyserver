import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Providers({ api, navigate }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const refresh = () => api.listProviders().then(setItems).catch(err => setError(err.message || '服务商读取失败'));
  useEffect(() => {
    const controller = new AbortController();
    api.listProviders({ signal: controller.signal }).then(setItems)
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '服务商读取失败'); });
    return () => controller.abort();
  }, [api]);
  const remove = async id => { if (!confirm('删除此 ACME 服务商？')) return; try { await api.deleteProvider(id); await refresh(); } catch (err) { setError(err.message || '删除失败'); } };
  return html`<div><div class="page-toolbar"><h2>ACME 服务商</h2><button class="button button-primary" onclick=${() => navigate('/providers/new')}>添加服务商</button></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>Directory URL</th><th>状态</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.name}</td><td class="mono">${item.directory_url}</td><td><span class=${`status-pill ${item.status === 'active' ? 'running' : ''}`}>${item.status}</span></td><td><button class="button" onclick=${() => navigate(`/providers/${item.id}/register`)}>注册账户</button> <button class="button button-danger" onclick=${() => remove(item.id)}>删除</button></td></tr>`)}</tbody>
    </table>${!items.length && !error ? html`<div class="empty-state">没有服务商</div>` : null}</div>
  </div>`;
}
