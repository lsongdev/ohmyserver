import { html } from '@panel/shared/ui.js';
import { useCallback, useEffect, useState } from 'preact/hooks';

function displayName(names = []) { return names[0]?.replace(/^\//, '') || '(unnamed)'; }

export default function Containers({ api, navigate }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.listContainers({ signal: controller.signal }).then(data => setItems(data || []))
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '容器读取失败'); });
    return () => controller.abort();
  }, [api, refresh]);
  const act = useCallback(async (id, action) => {
    setBusy(`${id}:${action}`);
    setError('');
    try {
      if (action === 'delete' && !confirm('确定删除此容器吗？')) return;
      if (action === 'delete') await api.deleteContainer(id);
      else await api[`${action}Container`](id);
      setRefresh(value => value + 1);
    } catch (err) { setError(err.message || '操作失败'); }
    finally { setBusy(''); }
  }, [api]);
  return html`<div>
    <div class="page-toolbar"><h2>容器</h2><div class="toolbar-actions"><button class="button button-secondary" onclick=${() => navigate('/docker/containers/new')}>创建容器</button><button class="button" onclick=${() => setRefresh(value => value + 1)}>刷新</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>镜像</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}>
        <td><a class="resource-link" href=${`#/docker/containers/${encodeURIComponent(item.id)}`}>${displayName(item.names)}</a><div class="muted mono">${item.id.slice(0, 12)}</div></td>
        <td>${item.image}</td><td><span class=${`status-pill ${item.state}`}>${item.status || item.state}</span></td>
        <td>${item.created ? new Date(item.created * 1000).toLocaleString() : '—'}</td>
        <td class="table-actions">${item.state === 'running'
          ? html`<button class="button button-link action-link" disabled=${busy === `${item.id}:restart`} onclick=${() => act(item.id, 'restart')}>重启</button><button class="button button-link action-link" disabled=${busy === `${item.id}:stop`} onclick=${() => act(item.id, 'stop')}>停止</button>`
          : html`<button class="button button-link action-link" disabled=${busy === `${item.id}:start`} onclick=${() => act(item.id, 'start')}>启动</button>`}
          <button class="button button-link button-danger action-link" disabled=${busy === `${item.id}:delete`} onclick=${() => act(item.id, 'delete')}>删除</button>
        </td></tr>`)}</tbody></table>
      ${!items.length && !error ? html`<div class="empty-state">没有容器</div>` : null}
    </div>
  </div>`;
}
