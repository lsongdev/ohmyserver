import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';
import { HostForm } from '../forms.js';

const payload = value => ({ ...value, certificate_id: value.certificate_id ? Number(value.certificate_id) : null });
export default function EditHost({ api, params, navigate }) {
  const [value, setValue] = useState(null), [listeners, setListeners] = useState([]), [policies, setPolicies] = useState([]), [certificates, setCertificates] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => { const c = new AbortController(); Promise.all([api.hosts({signal:c.signal}), api.listeners({signal:c.signal}), api.policies({signal:c.signal}), api.certificates({signal:c.signal})]).then(([items,l,p,certs]) => { setValue((items||[]).find(x => String(x.id) === String(params.id)) || null); setListeners(l||[]); setPolicies(p||[]); setCertificates(certs||[]); }).catch(e => { if(e.name !== 'AbortError') setError(e.message); }); return () => c.abort(); }, [api, params.id]);
  const submit = async draft => { setBusy(true); setError(''); try { await api.updateHost(params.id, payload(draft)); navigate('/hosts'); } catch(e) { setError(e.message || '保存 Virtual Host 失败'); } finally { setBusy(false); } };
  return html`<section><div class="page-toolbar"><h2>编辑 Virtual Host</h2><button class="button" onClick=${() => navigate('/hosts')}>返回列表</button></div>${error && html`<div class="notice-card error" role="alert">${error}</div>`}${value ? html`<${HostForm} value=${{...value,certificate_id:value.certificate_id||'',listener_ids:value.listener_ids||[],policy_ids:value.policy_ids||[]}} listeners=${listeners} policies=${policies} certificates=${certificates} busy=${busy} onSubmit=${submit}/>` : null}</section>`;
}
