import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';
import { ListenerForm } from '../forms.js';

const payload = value => ({ ...value, certificate_id: value.certificate_id ? Number(value.certificate_id) : null });
export default function EditListener({ api, params, navigate }) {
  const [value, setValue] = useState(null), [policies, setPolicies] = useState([]), [certificates, setCertificates] = useState([]), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => { const c = new AbortController(); Promise.all([api.listeners({signal:c.signal}), api.policies({signal:c.signal}), api.certificates({signal:c.signal})]).then(([items,p,certs]) => { setValue((items||[]).find(x => String(x.id) === String(params.id)) || null); setPolicies(p||[]); setCertificates(certs||[]); }).catch(e => { if(e.name !== 'AbortError') setError(e.message); }); return () => c.abort(); }, [api, params.id]);
  const submit = async draft => { setBusy(true); setError(''); try { await api.updateListener(params.id, payload(draft)); navigate('/listeners'); } catch(e) { setError(e.message || '保存 Listener 失败'); } finally { setBusy(false); } };
  return html`<section><div class="page-toolbar"><h2>编辑 Listener</h2><button class="button" onClick=${() => navigate('/listeners')}>返回列表</button></div>${error && html`<div class="notice-card error" role="alert">${error}</div>`}${value ? html`<${ListenerForm} value=${{...value,certificate_id:value.certificate_id||'',policy_ids:value.policy_ids||[]}} policies=${policies} certificates=${certificates} busy=${busy} onSubmit=${submit}/>` : null}</section>`;
}
