import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Certificates({ api, navigate, href }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.listCertificates({ signal: controller.signal })
      .then(data => setItems(data || []))
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '证书列表读取失败'); });
    return () => controller.abort();
  }, [api, reload]);
  return html`<div>
    <div class="page-toolbar"><h2>证书</h2><div class="button-group"><button class="button button-primary" onclick=${() => navigate('/certificates/new')}>申请证书</button><button class="button" onclick=${() => navigate('/certificates/csr')}>生成 CSR</button><button class="button" onclick=${() => setReload(value => value + 1)}>刷新</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>域名</th><th>状态</th><th>有效期</th><th>创建时间</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}>
        <td><a href=${href(`/certificates/${item.id}`)}>${item.common_name || item.domains?.[0] || `证书 #${item.id}`}</a><div class="muted">${(item.dns_names?.length ? item.dns_names : item.domains || []).join(', ')}</div></td>
        <td><span class=${`status-pill ${item.status === 'active' ? 'running' : ''}`}>${item.status}</span></td>
        <td>${item.days_until_expiry >= 0 ? `${item.days_until_expiry} 天 · ${new Date(item.expires_at).toLocaleDateString()}` : '尚未签发'}</td>
        <td>${item.created_at || '—'}</td><td>${item.status === 'active' ? html`<a class="button" href=${api.certificateDownloadURL(item.id)}>下载</a>` : '—'}</td>
      </tr>`)}</tbody></table>
      ${!items.length && !error ? html`<div class="empty-state">暂无证书</div>` : null}
    </div>
  </div>`;
}
