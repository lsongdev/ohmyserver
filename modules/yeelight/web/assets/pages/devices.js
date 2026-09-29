import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function Devices({ api, navigate, href }) {
  const [items, setItems] = useState([]);
  const [found, setFound] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.list({ signal: controller.signal }).then(setItems).catch(e => { if (e.name !== 'AbortError') setError(e.message); });
    return () => controller.abort();
  }, [api, refresh]);
  const discover = async () => {
    setBusy(true); setError('');
    try { setFound(await api.discover()); } catch (e) { setError(e.message); } finally { setBusy(false); }
  };
  const add = async device => {
    try { await api.create(device); setFound(value => value.filter(item => item.host !== device.host)); setRefresh(value => value + 1); }
    catch (e) { setError(e.message); }
  };
  const remove = async id => {
    if (!confirm('确定删除该灯具配置？')) return;
    try { await api.remove(id); setRefresh(value => value + 1); } catch (e) { setError(e.message); }
  };
  return html`<div>
    <div class="page-toolbar"><h2>智能灯具</h2><div class="button-group"><button class="button" disabled=${busy} onClick=${discover}>${busy ? '搜索中…' : '发现设备'}</button><button class="button button-primary" onClick=${() => navigate('/new')}>手动添加</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    ${found.length ? html`<section class="section"><h3>发现的设备</h3><div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>地址</th><th>操作</th></tr></thead><tbody>${found.map(device => html`<tr key=${device.host}><td>${device.name || device.host}</td><td>${device.host}:${device.port}</td><td><button class="button button-primary" onClick=${() => add(device)}>添加</button></td></tr>`)}</tbody></table></div></section>` : null}
    ${items.length ? html`<div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>地址</th><th>状态</th><th>最近在线</th><th>操作</th></tr></thead><tbody>${items.map(device => html`<tr key=${device.id}><td><a href=${href(`/${device.id}`)}>${device.name}</a></td><td>${device.host}:${device.port}</td><td><span class=${`status-pill ${device.status === 'online' ? 'running' : ''}`}>${device.status}</span></td><td>${device.last_seen ? new Date(device.last_seen).toLocaleString() : '从未'}</td><td><div class="button-group"><button class="button" onClick=${() => navigate(`/${device.id}/edit`)}>编辑</button><button class="button button-danger" onClick=${() => remove(device.id)}>删除</button></div></td></tr>`)}</tbody></table></div>` : html`<div class="empty-state">尚未配置 Yeelight 灯具，可搜索局域网设备或手动添加。</div>`}
  </div>`;
}
