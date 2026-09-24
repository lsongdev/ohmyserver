import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Servers({ api, navigate }) {
  const [items, setItems] = useState([]); const [message, setMessage] = useState(''); const [busy, setBusy] = useState(false); const [refresh, setRefresh] = useState(0);
  useEffect(() => { const c = new AbortController(); api.list({ signal: c.signal }).then(setItems).catch(e => { if (e.name !== 'AbortError') setMessage(e.message); }); return () => c.abort(); }, [api, refresh]);
  const test = async id => { setBusy(true); setMessage(''); try { await api.test(id); setMessage('连接成功'); } catch (e) { setMessage(e.message); } finally { setBusy(false); } };
  const remove = async id => { if (!confirm('确定删除此 iDRAC 配置？')) return; setBusy(true); try { await api.remove(id); setRefresh(v => v + 1); } catch (e) { setMessage(e.message); } finally { setBusy(false); } };
  return html`<div><div class="page-toolbar"><h2>iDRAC 服务器</h2><button class="button button-primary" onclick=${() => navigate('/idrac/new')}>添加服务器</button></div>${message ? html`<div class="notice-card"><p>${message}</p></div>` : null}<div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>主机</th><th>用户</th><th>版本</th><th>凭据</th><th>操作</th></tr></thead><tbody>${items.map(v => html`<tr key=${v.id}><td><a href=${`#/idrac/${v.id}`}>${v.name}</a></td><td>${v.host}:${v.port}</td><td>${v.username}</td><td>${v.version}</td><td>${v.has_credential ? '已配置' : '缺失'}</td><td><div class="button-group"><button class="button" disabled=${busy} onclick=${() => test(v.id)}>测试</button><button class="button" onclick=${() => navigate(`/idrac/${v.id}/edit`)}>编辑</button><button class="button button-danger" disabled=${busy} onclick=${() => remove(v.id)}>删除</button></div></td></tr>`)}</tbody></table>${!items.length ? html`<div class="empty-state">暂无 iDRAC 服务器配置</div>` : null}</div></div>`;
}
