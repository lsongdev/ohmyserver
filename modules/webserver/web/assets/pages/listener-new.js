import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';
import { ListenerForm } from '../forms.js';

const blank = { name: '', address: ':8080', protocol: 'http', enabled: true, min_tls_version: 'TLS1.2', trusted_proxies: '', certificate_id: '', policy_ids: [] };
const payload = value => ({ ...value, certificate_id: value.certificate_id ? Number(value.certificate_id) : null });
export default function NewListener({ api, navigate }) {
  const [policies, setPolicies] = useState([]), [certificates, setCertificates] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => { const c = new AbortController(); Promise.all([api.policies({ signal: c.signal }), api.certificates({ signal: c.signal })]).then(([p, certs]) => { setPolicies(p || []); setCertificates(certs || []); }).catch(e => { if (e.name !== 'AbortError') setError(e.message || '读取 Web Server 配置失败'); }); return () => c.abort(); }, [api]);
  const submit = async value => { setBusy(true); setError(''); try { await api.createListener(payload(value)); navigate('/listeners'); } catch (e) { setError(e.message || '创建 Listener 失败'); } finally { setBusy(false); } };
  return html`<section class="section"><div class="page-toolbar"><h2>添加 Listener</h2><button class="button" onClick=${() => navigate('/listeners')}>返回列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<${ListenerForm} value=${blank} policies=${policies} certificates=${certificates} busy=${busy} submitLabel="创建 Listener" onSubmit=${submit}/></section>`;
}
