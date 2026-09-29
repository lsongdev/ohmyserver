import { html } from '@module/ui';
import { useState } from 'preact/hooks';

export default function NewProvider({ api, navigate }) {
  const [form, setForm] = useState({ name: '', type: 'cloudflare', config: '{}' }), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); let config; try { config = JSON.parse(form.config); } catch { setError('凭据配置必须是有效 JSON'); return; } setBusy(true); setError(''); try { await api.createProvider({ ...form, config }); navigate('/providers'); } catch (e) { setError(e.message || '创建服务商失败'); } finally { setBusy(false); } };
  return html`<section><h2>添加 DNS 服务商连接</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="dns-provider-form" onSubmit=${submit}><label class="form-field">名称<input class="input" value=${form.name} onInput=${e => set('name', e.currentTarget.value)} required /></label><label class="form-field">类型<input class="input" value=${form.type} onInput=${e => set('type', e.currentTarget.value)} required /></label><label class="form-field">凭据 JSON<textarea class="input" value=${form.config} onInput=${e => set('config', e.currentTarget.value)} required /></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加连接'}</button><button class="button" type="button" onClick=${() => navigate('/providers')}>取消</button></div></form></section>`;
}
