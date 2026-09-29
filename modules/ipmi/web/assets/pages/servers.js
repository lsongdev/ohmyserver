import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function Servers({ api, navigate, href }) {
  const [items, setItems] = useState([]); const [error, setError] = useState(''); const [busy, setBusy] = useState(''); const [refresh, setRefresh] = useState(0);
  useEffect(() => { const controller = new AbortController(); api.listServers({ signal: controller.signal }).then(setItems).catch(err => { if (err.name !== 'AbortError') setError(err.message || 'IPMI 服务器列表读取失败'); }); return () => controller.abort(); }, [api, refresh]);
  const test = async id => { setBusy(`test:${id}`); setError(''); try { await api.testServer(id); setError('连接测试成功'); } catch (err) { setError(err.message || '连接测试失败'); } finally { setBusy(''); } };
  const remove = async id => { if (!confirm('确定删除此 IPMI 配置？')) return; setBusy(`delete:${id}`); try { await api.deleteServer(id); setRefresh(n => n + 1); } catch (err) { setError(err.message || '删除失败'); } finally { setBusy(''); } };
  return html`<div><div class="page-toolbar"><h2>BMC 服务器</h2><div class="button-group"><button class="button button-primary" onclick=${() => navigate('/new')}>添加 IPMI</button><button class="button" onclick=${() => setRefresh(n => n + 1)}>刷新</button></div></div>
    ${error ? html`<div class=${`notice-card ${error.includes('成功') ? '' : 'error'}`} role="status"><p>${error}</p></div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>BMC 地址</th><th>用户</th><th>凭据</th><th>描述</th><th>操作</th></tr></thead><tbody>${items.map(item => html`<tr key=${item.id}><td><a href=${href(`/${item.id}`)}>${item.name}</a></td><td class="mono">${item.bmc_ip}:${item.port}</td><td>${item.username}</td><td>${item.has_credential ? '已配置' : '缺失'}</td><td>${item.description || '—'}</td><td><div class="button-group"><button class="button" disabled=${!!busy} onclick=${() => test(item.id)}>${busy === `test:${item.id}` ? '测试中…' : '测试'}</button><button class="button" onclick=${() => navigate(`/${item.id}/edit`)}>编辑</button><button class="button button-danger" disabled=${!!busy} onclick=${() => remove(item.id)}>删除</button></div></td></tr>`)}</tbody></table>${!items.length && !error ? html`<div class="empty-state">暂无 IPMI 服务器</div>` : null}</div>
  </div>`;
}
