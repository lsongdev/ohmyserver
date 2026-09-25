import { useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const blank = () => ({ name: '', type: 'http', target: '', port: 443, interval: 60, timeout: 10, expected_status: 200, enabled: true });
export default function NewMonitor({ api, navigate }) {
  const [draft, setDraft] = useState(blank()), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const field = (name, value) => setDraft(current => ({ ...current, [name]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.save(draft); navigate('/'); } catch (e) { setError(e.message || '添加监测目标失败'); } finally { setBusy(false); } };
  return html`<section><h2>添加监测目标</h2>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <form class="uptime-form" onSubmit=${submit}><label class="form-field">名称<input class="input" required value=${draft.name} onInput=${e => field('name', e.currentTarget.value)} /></label><label class="form-field">类型<select class="input" value=${draft.type} onChange=${e => field('type', e.currentTarget.value)}><option value="http">HTTP</option><option value="tcp">TCP</option><option value="ping">Ping</option></select></label><label class="form-field">目标<input class="input" required value=${draft.target} onInput=${e => field('target', e.currentTarget.value)} placeholder="https://example.com" /></label>${draft.type === 'tcp' ? html`<label class="form-field">端口<input class="input" type="number" min="1" max="65535" value=${draft.port} onInput=${e => field('port', Number(e.currentTarget.value))} /></label>` : null}<label class="form-field">间隔（秒）<input class="input" type="number" min="10" value=${draft.interval} onInput=${e => field('interval', Number(e.currentTarget.value))} /></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加目标'}</button><button class="button" type="button" onClick=${() => navigate('/')}>取消</button></div></form>
  </section>`;
}
