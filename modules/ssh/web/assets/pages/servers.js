import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function Servers({ api, navigate, hrefTo }) {
  const [items, setItems] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const load = async () => { try { setItems(await api.list()); } catch (e) { setError(e.message || 'SSH 服务器读取失败'); } };
  useEffect(() => { load(); }, [api]);
  const remove = async id => { if (!confirm('删除此 SSH 服务器配置？')) return; setBusy(true); try { await api.remove(id); await load(); } catch (e) { setError(e.message || '删除失败'); } finally { setBusy(false); } };
  const terminalHref = id => hrefTo?.('terminal', `/ssh/${id}`);
  return html`<div class="ssh-page"><section class="card"><div class="page-toolbar"><h2>SSH 服务器</h2><div class="button-group"><button class="button button-primary" onClick=${() => navigate('/servers/new')}>添加服务器</button><button class="button" onClick=${load} disabled=${busy}>刷新</button></div></div>
    ${error && html`<div class="notice-card error" role="alert">${error}</div>`}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>地址</th><th>用户</th><th>认证</th><th>状态</th><th>操作</th></tr></thead><tbody>${items.map(item => html`<tr key=${item.id}><td><strong>${item.name}</strong><div class="muted">${item.description || ''}</div></td><td class="mono">${item.host}:${item.port}</td><td>${item.username}</td><td>${item.auth_type}${item.has_credential ? '' : ' · 凭据缺失'}</td><td>${item.enabled ? '启用' : '停用'}</td><td><div class="button-group">${item.enabled && item.has_credential && terminalHref(item.id) ? html`<a class="button" href=${terminalHref(item.id)}>终端</a>` : null}<button class="button" onClick=${() => navigate(`/servers/${item.id}`)}>监控</button><button class="button" onClick=${() => navigate(`/servers/${item.id}/edit`)}>编辑</button><button class="button button-danger" disabled=${busy} onClick=${() => remove(item.id)}>删除</button></div></td></tr>`)}</tbody></table>${items.length === 0 ? html`<div class="empty-state">尚无 SSH 服务器配置</div>` : null}</div>
  </section></div>`;
}
