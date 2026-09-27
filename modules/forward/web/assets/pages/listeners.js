import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Listeners({ api, navigate }) {
  const [items, setItems] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const load = async (signal) => setItems(await api.list({ signal }));
  useEffect(() => { const controller = new AbortController(); load(controller.signal).catch(e => { if (e.name !== 'AbortError') setError(e.message); }); return () => controller.abort(); }, [api]);
  const run = async (fn) => { setBusy(true); setError(''); try { await fn(); await load(); } catch (e) { setError(e.message || '操作失败'); } finally { setBusy(false); } };
  return html`<section>
    <div class="page-toolbar"><h2>TCP/UDP 转发</h2><div class="button-group"><button class="button button-primary" onClick=${() => navigate('/new')}>添加转发</button><button class="button" disabled=${busy} onClick=${() => run(() => load())}>刷新</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>协议</th><th>监听</th><th>转发目标</th><th>状态</th><th>操作</th></tr></thead>
    <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.name}</td><td>${item.protocol.toUpperCase()}</td><td>:${item.listen_port}</td><td>${item.forward_host}:${item.forward_port}</td><td>${item.status}</td><td class="table-actions">${item.status === 'active' ? html`<button class="button" disabled=${busy} onClick=${() => run(() => api.action(item.id, 'deactivate'))}>停用</button>` : html`<button class="button" disabled=${busy} onClick=${() => run(() => api.action(item.id, 'activate'))}>启用</button>`}<button class="button" disabled=${busy} onClick=${() => run(() => api.action(item.id, 'restart'))}>重启</button><button class="button button-danger" disabled=${busy} onClick=${() => run(() => api.remove(item.id))}>删除</button></td></tr>`)}</tbody>
    </table>${!items.length ? html`<div class="empty-state">暂无转发监听器</div>` : null}</div>
  </section>`;
}
