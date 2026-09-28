import { useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function NewSetting({ api, navigate }) {
  const [form, setForm] = useState({ name: '', type: 'string', value: '', description: '' });
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.create(form); navigate('/'); } catch (e) { setError(e.message || '创建失败'); } finally { setBusy(false); } };
  return html`<section><h2>新增设置</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <form class="settings-create" onSubmit=${submit}><label class="form-field"><span>名称</span><input class="input" required value=${form.name} onInput=${e => set('name', e.currentTarget.value)}/></label><label class="form-field"><span>类型</span><select class="input" value=${form.type} onChange=${e => set('type', e.currentTarget.value)}><option value="string">文本</option><option value="number">数字</option><option value="boolean">布尔</option><option value="json">JSON</option></select></label><label class="form-field"><span>值</span><input class="input" required value=${form.value} onInput=${e => set('value', e.currentTarget.value)}/></label><label class="form-field"><span>说明</span><input class="input" value=${form.description} onInput=${e => set('description', e.currentTarget.value)}/></label><div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy}>${busy ? '创建中…' : '创建'}</button><button class="button" type="button" onClick=${() => navigate('/')}>取消</button></div></form>
  </section>`;
}
