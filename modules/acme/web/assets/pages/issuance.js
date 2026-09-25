import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Issuance({ api, params, navigate }) {
  const [item, setItem] = useState(null); const [error, setError] = useState('');
  useEffect(() => {
    let alive = true; let timer;
    const load = () => api.getIssuance(params.id).then(value => { if (!alive) return; setItem(value); setError(''); if (value.status === 'queued' || value.status === 'running') timer = setTimeout(load, 2000); }).catch(err => { if (alive) setError(err.message || '签发状态读取失败'); });
    load(); return () => { alive = false; clearTimeout(timer); };
  }, [api, params.id]);
  return html`<div><div class="page-toolbar"><h2>证书签发</h2><button class="button" onclick=${() => navigate('/issuances')}>返回签发记录</button></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    ${item ? html`<section class="section"><dl><dt>域名</dt><dd>${item.domains}</dd><dt>状态</dt><dd><span class=${`status-pill ${item.status === 'succeeded' ? 'running' : ''}`}>${item.status}</span></dd><dt>当前步骤</dt><dd>${item.step}</dd><dt>验证方式</dt><dd>${item.challenge_type}</dd><dt>证书 ID</dt><dd>${item.certificate_id}</dd><dt>错误</dt><dd>${item.error || '—'}</dd></dl>${item.status === 'succeeded' ? html`<button class="button button-primary" onclick=${() => navigate('/certificates')}>查看证书</button>` : null}</section>` : !error ? html`<div class="loading-state">正在读取签发状态…</div>` : null}
  </div>`;
}
