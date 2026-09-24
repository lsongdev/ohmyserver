import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Accounts({ api }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const refresh = () => api.listAccounts().then(setItems).catch(err => setError(err.message || '账户读取失败'));
  useEffect(() => {
    const controller = new AbortController();
    api.listAccounts({ signal: controller.signal }).then(setItems)
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '账户读取失败'); });
    return () => controller.abort();
  }, [api]);
  const remove = async id => { if (!confirm('删除此 ACME 账户？')) return; try { await api.deleteAccount(id); await refresh(); } catch (err) { setError(err.message || '删除失败'); } };
  return html`<div><div class="page-toolbar"><h2>ACME 账户</h2></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>服务商</th><th>邮箱</th><th>账户 URL</th><th>状态</th><th>条款</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.provider_name}</td><td>${item.email}</td><td class="mono">${item.account_url}</td><td><span class=${`status-pill ${item.status === 'valid' ? 'running' : ''}`}>${item.status}</span></td><td>${item.agreed_terms ? '已同意' : '未同意'}</td><td><button class="button button-danger" onclick=${() => remove(item.id)}>删除</button></td></tr>`)}</tbody>
    </table>${!items.length && !error ? html`<div class="empty-state">没有 ACME 账户</div>` : null}</div>
  </div>`;
}
