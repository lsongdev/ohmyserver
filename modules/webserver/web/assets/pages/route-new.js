import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';
import { RouteForm } from '../forms.js';

const empty = { name: '', enabled: true, method: '', path: '/', handler_type: 'reverse_proxy', upstream_url: '', policy_ids: [], request_timeout_seconds: 0, response_header_timeout_seconds: 0, max_conns_per_host: 0, response_status: 200, redirect_status: 302 };
const numeric = value => ({ ...value, request_timeout_seconds: Number(value.request_timeout_seconds), response_header_timeout_seconds: Number(value.response_header_timeout_seconds), max_conns_per_host: Number(value.max_conns_per_host), response_status: Number(value.response_status), redirect_status: Number(value.redirect_status) });
export default function NewRoute({ api, params, navigate }) {
  const [host, setHost] = useState(null), [policies, setPolicies] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => { const c = new AbortController(); Promise.all([api.hosts({ signal: c.signal }), api.policies({ signal: c.signal })]).then(([hosts, p]) => { setHost((hosts || []).find(item => String(item.id) === String(params.id)) || null); setPolicies(p || []); }).catch(e => { if (e.name !== 'AbortError') setError(e.message || '读取 Virtual Host 失败'); }); return () => c.abort(); }, [api, params.id]);
  const submit = async value => { setBusy(true); setError(''); try { await api.createRoute(params.id, numeric(value)); navigate('/hosts'); } catch (e) { setError(e.message || '创建 Route 失败'); } finally { setBusy(false); } };
  return html`<section class="section"><div class="page-toolbar"><h2>为 ${host?.name || 'Virtual Host'} 添加 Route</h2><button class="button" onClick=${() => navigate('/hosts')}>返回 Virtual Hosts</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<${RouteForm} value=${empty} policies=${policies} busy=${busy || !host} submitLabel="创建 Route" onSubmit=${submit}/></section>`;
}
