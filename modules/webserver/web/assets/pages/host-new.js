import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';
import { HostForm } from '../forms.js';

const empty = { name: '', hostnames: '', enabled: true, certificate_id: '', force_https: false, listener_ids: [], policy_ids: [] };
const payload = value => ({ ...value, certificate_id: value.certificate_id ? Number(value.certificate_id) : null });
export default function NewHost({ api, navigate }) {
  const [listeners, setListeners] = useState([]), [policies, setPolicies] = useState([]), [certificates, setCertificates] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => { const c = new AbortController(); Promise.all([api.listeners({ signal: c.signal }), api.policies({ signal: c.signal }), api.certificates({ signal: c.signal })]).then(([l, p, certs]) => { setListeners(l || []); setPolicies(p || []); setCertificates(certs || []); }).catch(e => { if (e.name !== 'AbortError') setError(e.message || '读取 Web Server 配置失败'); }); return () => c.abort(); }, [api]);
  const submit = async value => { setBusy(true); setError(''); try { await api.createHost(payload(value)); navigate('/webserver/hosts'); } catch (e) { setError(e.message || '创建 Virtual Host 失败'); } finally { setBusy(false); } };
  return html`<section class="section"><div class="page-toolbar"><h2>添加 Virtual Host</h2><button class="button" onClick=${() => navigate('/webserver/hosts')}>返回列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<${HostForm} value=${empty} listeners=${listeners} policies=${policies} certificates=${certificates} busy=${busy} submitLabel="创建 Virtual Host" onSubmit=${submit}/></section>`;
}
