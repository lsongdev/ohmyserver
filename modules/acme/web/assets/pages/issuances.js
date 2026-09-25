import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Issuances({ api, navigate }) {
  const [items, setItems] = useState([]); const [error, setError] = useState('');
  useEffect(() => { const controller = new AbortController(); api.listIssuances({ signal: controller.signal }).then(setItems).catch(err => { if (err.name !== 'AbortError') setError(err.message || '签发记录读取失败'); }); return () => controller.abort(); }, [api]);
  return html`<div><div class="page-toolbar"><h2>签发记录</h2><button class="button button-primary" onclick=${() => navigate('/certificates/new')}>申请证书</button></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>域名</th><th>验证方式</th><th>状态</th><th>步骤</th><th>提交时间</th></tr></thead><tbody>${items.map(item => html`<tr key=${item.id} onclick=${() => navigate(`/issuances/${item.id}`)}><td>${item.domains}</td><td>${item.challenge_type}</td><td>${item.status}</td><td>${item.step}</td><td>${new Date(item.created_at).toLocaleString()}</td></tr>`)}</tbody></table>${!items.length && !error ? html`<div class="empty-state">暂无签发记录</div>` : null}</div>
  </div>`;
}
