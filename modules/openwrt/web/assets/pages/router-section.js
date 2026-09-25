import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

const labels = { overview: '概览', network: '网络与 Wi-Fi', dhcp: 'DHCP 租约', packages: '软件包', firewall: '防火墙', services: '服务' };
function Value({ value }) {
  if (value == null) return html`<span class="muted">—</span>`;
  if (typeof value !== 'object') return String(value);
  if (Array.isArray(value)) return value.length ? html`<div class="data-table-wrap"><table class="data-table"><thead><tr>${Object.keys(value[0] || {}).map(key => html`<th key=${key}>${key}</th>`)}</tr></thead><tbody>${value.map((row, index) => html`<tr key=${index}>${Object.keys(value[0] || {}).map(key => html`<td key=${key}><${Value} value=${row[key]} /></td>`)}</tr>`)}</tbody></table></div>` : html`<div class="empty-state">暂无数据</div>`;
  return html`<dl class="openwrt-values">${Object.entries(value).map(([key, item]) => html`<div key=${key}><dt>${key.replaceAll('_', ' ')}</dt><dd><${Value} value=${item} /></dd></div>`)}</dl>`;
}

export default function RouterSection({ api, params, navigate, href }) {
  const { id, section = 'overview' } = params;
  const [router, setRouter] = useState(null); const [data, setData] = useState(null); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const load = () => {
    const controller = new AbortController();
    Promise.all([api.getRouter(id, { signal: controller.signal }), api.readSection(id, section, { signal: controller.signal })]).then(([a, b]) => { setRouter(a); setData(b); }).catch(e => { if (e.name !== 'AbortError') setError(e.message); });
    return () => controller.abort();
  };
  useEffect(load, [api, id, section]);
  const action = async payload => { if (payload.action === 'reboot' && !confirm('确定重启此路由器？')) return; setBusy(true); setError(''); try { await api.action(id, payload); } catch (e) { setError(e.message); } finally { setBusy(false); } };
  const testConnection = async () => { setBusy(true); setError(''); try { await api.testRouter(id); alert('连接成功'); } catch (e) { setError(e.message); } finally { setBusy(false); } };
  const remove = async () => { if (!confirm(`删除路由器 ${router?.name}？`)) return; try { await api.deleteRouter(id); navigate('/'); } catch (e) { setError(e.message); } };
  return html`<div class="page-toolbar"><div><h2>${router?.name || 'OpenWrt 路由器'}</h2><span class="muted">${router ? `${router.host}:${router.port}` : ''}</span></div><div class="form-actions"><button class="button button-secondary" disabled=${busy} onClick=${testConnection}>测试连接</button><button class="button" disabled=${busy} onClick=${() => action({ action: 'reboot' })}>重启</button><a class="button button-secondary" href=${href(`/routers/${id}/edit`)}>编辑</a><button class="button button-danger" onClick=${remove}>删除</button></div></div>
    <nav class="module-nav">${Object.entries(labels).map(([key, label]) => html`<a class=${section === key ? 'active' : ''} href=${href(`/routers/${id}/${key}`)}>${label}</a>`)}</nav>
    ${error ? html`<div class="notice-card error">${error}</div>` : null}
    ${section === 'services' && Array.isArray(data) ? html`<div class="card"><h3>服务操作</h3><div class="form-actions">${data.map(item => html`<button class="button button-secondary" disabled=${busy} onClick=${() => action({ action: 'restart_service', service: item.name })}>重启 ${item.name}</button>`)}</div></div>` : null}
    <div class="card"><h3>${labels[section]}</h3>${data == null ? html`<p class="muted">读取中…</p>` : html`<${Value} value=${data} />`}</div>`;
}
