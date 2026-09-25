import { useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const blank = { name: '', protocol: 'tcp', listen_port: '', forward_host: '127.0.0.1', forward_port: '', status: 'active' };
export default function NewListener({ api, navigate }) {
  const [draft, setDraft] = useState(blank), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const set = (key, value) => setDraft(current => ({ ...current, [key]: value }));
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try { await api.create({ ...draft, listen_port: Number(draft.listen_port), forward_port: Number(draft.forward_port) }); navigate('/'); }
    catch (e) { setError(e.message || '创建转发失败'); } finally { setBusy(false); }
  };
  return html`<section><h2>添加转发</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <form class="forward-form" onSubmit=${submit}>
      <label class="form-field">名称<input class="input" required value=${draft.name} onInput=${e => set('name', e.currentTarget.value)} /></label>
      <label class="form-field">协议<select class="input" value=${draft.protocol} onChange=${e => set('protocol', e.currentTarget.value)}><option value="tcp">TCP</option><option value="udp">UDP</option></select></label>
      <label class="form-field">监听端口<input class="input" type="number" min="1" max="65535" required value=${draft.listen_port} onInput=${e => set('listen_port', e.currentTarget.value)} /></label>
      <label class="form-field">目标主机<input class="input" required value=${draft.forward_host} onInput=${e => set('forward_host', e.currentTarget.value)} /></label>
      <label class="form-field">目标端口<input class="input" type="number" min="1" max="65535" required value=${draft.forward_port} onInput=${e => set('forward_port', e.currentTarget.value)} /></label>
      <div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '创建中…' : '创建转发'}</button><button class="button" type="button" onClick=${() => navigate('/')}>取消</button></div>
    </form></section>`;
}
