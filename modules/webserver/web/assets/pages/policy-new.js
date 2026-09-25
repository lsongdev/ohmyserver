import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';
import { PolicyForm } from '../forms.js';

const empty = { name: '', max_body_bytes: 0, request_timeout_seconds: 0, rate_requests_per_second: 0, rate_burst: 0, rate_key: 'client_ip', rate_max_keys: 10000, rate_idle_timeout_seconds: 600, concurrency_max: 0 };
const numeric = value => ({ ...value, max_body_bytes: Number(value.max_body_bytes), request_timeout_seconds: Number(value.request_timeout_seconds), rate_requests_per_second: Number(value.rate_requests_per_second), rate_burst: Number(value.rate_burst), rate_max_keys: Number(value.rate_max_keys), rate_idle_timeout_seconds: Number(value.rate_idle_timeout_seconds), concurrency_max: Number(value.concurrency_max) });
export default function NewPolicy({ api, navigate }) {
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const submit = async value => { setBusy(true); setError(''); try { await api.createPolicy(numeric(value)); navigate('/policies'); } catch (e) { setError(e.message || '创建 Policy 失败'); } finally { setBusy(false); } };
  return html`<section class="section"><div class="page-toolbar"><h2>添加 Policy</h2><button class="button" onClick=${() => navigate('/policies')}>返回列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<${PolicyForm} value=${empty} busy=${busy} submitLabel="创建 Policy" onSubmit=${submit}/></section>`;
}
