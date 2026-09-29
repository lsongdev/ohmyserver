import { html } from '@module/ui';
import { useState } from 'preact/hooks';

export default function NewProvider({ api, navigate }) {
  const [name, setName] = useState(''), [url, setURL] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createProvider({ name, directory_url: url }); navigate('/providers'); } catch (e) { setError(e.message || '创建服务商失败'); } finally { setBusy(false); } };
  return html`<div><h2>添加 ACME 服务商</h2>${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}<form class="section" onSubmit=${submit}><div class="form-group"><label>名称</label><input class="input" value=${name} onInput=${e => setName(e.currentTarget.value)} required /></div><div class="form-group"><label>ACME Directory URL</label><input class="input" type="url" value=${url} onInput=${e => setURL(e.currentTarget.value)} required /></div><div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy}>${busy ? '添加中…' : '添加'}</button><button class="button" type="button" onclick=${() => navigate('/providers')}>取消</button></div></form></div>`;
}
