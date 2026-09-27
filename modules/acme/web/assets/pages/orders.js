import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Orders({ api }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    api.listOrders({ signal: controller.signal }).then(setItems)
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '订单读取失败'); });
    return () => controller.abort();
  }, [api]);
  return html`<div><div class="page-toolbar"><h2>ACME 订单</h2></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>域名</th><th>账户 ID</th><th>Order URL</th><th>创建时间</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.domains}</td><td>${item.account_id}</td><td class="mono">${item.order_url}</td><td>${item.created_at}</td></tr>`)}</tbody>
    </table>${!items.length && !error ? html`<div class="empty-state">没有 ACME 订单</div>` : null}</div>
  </div>`;
}
