import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';

const initial = { name: '', kind: 'block', url: '', enabled: true, refresh_hours: 24 };
export default function NewSubscription({ api, navigate }) {
  const [form, setForm] = useState(initial), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createSubscription(form); navigate('/filters'); } catch (e) { setError(e.message || '订阅添加失败'); } finally { setBusy(false); } };
  return html`<section class="card"><div class="page-toolbar"><h2>添加 DNS 订阅</h2><button class="button" onClick=${() => navigate('/filters')}>返回列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="dns-filter-form" onSubmit=${submit}><label class="form-field">名称<input class="input" value=${form.name} onInput=${e => set('name', e.currentTarget.value)} required /></label><label class="form-field">类型<select class="input" value=${form.kind} onChange=${e => set('kind', e.currentTarget.value)}><option value="block">拦截列表</option><option value="allow">允许列表</option></select></label><label class="form-field">URL<input class="input" type="url" value=${form.url} onInput=${e => set('url', e.currentTarget.value)} required /></label><label class="form-field">刷新间隔（小时）<input class="input" type="number" min="1" value=${form.refresh_hours} onInput=${e => set('refresh_hours', Number(e.currentTarget.value))} /></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加订阅'}</button><button class="button" type="button" onClick=${() => navigate('/filters')}>取消</button></div></form></section>`;
}
