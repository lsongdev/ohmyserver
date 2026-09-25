import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';

export default function NewZone({ api, navigate }) {
  const [form, setForm] = useState({ name: '', description: '', enabled: true }), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createZone(form); navigate('/zones'); } catch (e) { setError(e.message || '创建区域失败'); } finally { setBusy(false); } };
  return html`<section><div class="page-toolbar"><h2>创建 DNS 区域</h2><button class="button" onClick=${() => navigate('/zones')}>返回区域列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="dns-zone-form" onSubmit=${submit}><label class="form-field">区域名称<input class="input" placeholder="example.com" value=${form.name} onInput=${e => set('name', e.currentTarget.value)} required /></label><label class="form-field">说明<input class="input" value=${form.description} onInput=${e => set('description', e.currentTarget.value)} /></label><label class="dns-zone-enable"><input class="input dns-checkbox" type="checkbox" checked=${form.enabled} onChange=${e => set('enabled', e.currentTarget.checked)} />启用</label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '创建中…' : '创建区域'}</button><button class="button" type="button" onClick=${() => navigate('/zones')}>取消</button></div></form></section>`;
}
