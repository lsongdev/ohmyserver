import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function Routers({ api, href }) {
  const [routers, setRouters] = useState([]);
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    api.listRouters({ signal: controller.signal }).then(result => setRouters(result || [])).catch(e => { if (e.name !== 'AbortError') setError(e.message); });
    return () => controller.abort();
  }, [api]);
  return html`<div class="page-toolbar"><h2>已登记的路由器</h2><a class="button" href=${href('/routers/new')}>添加路由器</a></div>
    ${error ? html`<div class="notice-card error">${error}</div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>地址</th><th>协议</th><th>状态</th><th></th></tr></thead><tbody>
      ${routers.map(item => html`<tr key=${item.id}><td><a href=${href(`/routers/${item.id}`)}>${item.name}</a></td><td>${item.host}:${item.port}</td><td>${item.protocol}</td><td>${item.status}</td><td><a href=${href(`/routers/${item.id}`)}>打开</a></td></tr>`)}
      ${!routers.length ? html`<tr><td colspan="5" class="empty-state">尚未添加路由器</td></tr>` : null}
    </tbody></table></div>`;
}
