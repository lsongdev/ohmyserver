import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';

const blank = { mac: '', ip: '', hostname: '', description: '' };
export default function NewStaticLease({ api, navigate }) {
  const [form, setForm] = useState(blank), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createStaticLease(form); navigate('/dhcp/static-leases'); } catch (e) { setError(e.message || '创建静态租约失败'); } finally { setBusy(false); } };
  return html`<section><h2>添加静态租约</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="dhcp-reservation-form" onSubmit=${submit}><label class="form-field">MAC 地址<input class="input" value=${form.mac} onInput=${e => set('mac', e.currentTarget.value)} required /></label><label class="form-field">IPv4 地址<input class="input" value=${form.ip} onInput=${e => set('ip', e.currentTarget.value)} required /></label><label class="form-field">主机名<input class="input" value=${form.hostname} onInput=${e => set('hostname', e.currentTarget.value)} /></label><label class="form-field">描述<input class="input" value=${form.description} onInput=${e => set('description', e.currentTarget.value)} /></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加租约'}</button><button class="button" type="button" onClick=${() => navigate('/dhcp/static-leases')}>取消</button></div></form></section>`;
}
