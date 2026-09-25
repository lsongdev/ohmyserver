import { useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const empty = { name: '', type: '', config: '', sort_order: 0, enabled: true };
export default function NewWidget({ api, navigate }) {
  const [draft, setDraft] = useState(empty), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setDraft(current => ({ ...current, [key]: value }));
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try { await api.create({ ...draft, sort_order: Number(draft.sort_order) }); navigate('/'); }
    catch (e) { setError(e.message || '添加组件失败'); } finally { setBusy(false); }
  };
  return html`<section><h2>添加首页组件</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <form class="widget-form" onSubmit=${submit}><label class="form-field">名称<input class="input" required value=${draft.name} onInput=${e => set('name', e.currentTarget.value)}/></label><label class="form-field">类型<input class="input" required value=${draft.type} onInput=${e => set('type', e.currentTarget.value)} placeholder="如 system_info"/></label><label class="form-field">排序<input class="input" type="number" value=${draft.sort_order} onInput=${e => set('sort_order', e.currentTarget.value)}/></label><label class="form-field widget-config">配置<textarea class="input" rows="4" value=${draft.config} onInput=${e => set('config', e.currentTarget.value)}></textarea></label><label class="widget-enabled"><input class="input" type="checkbox" checked=${draft.enabled} onChange=${e => set('enabled', e.currentTarget.checked)}/> 启用</label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加组件'}</button><button class="button" type="button" onClick=${() => navigate('/')}>取消</button></div></form>
  </section>`;
}
