import { html } from '@module/ui';
import { useState } from 'preact/hooks';

export default function NewRule({ api, navigate }) {
  const [rule, setRule] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createRule(rule); navigate('/filters'); } catch (e) { setError(e.message || '规则添加失败'); } finally { setBusy(false); } };
  return html`<section><div class="page-toolbar"><h2>添加 DNS 自定义规则</h2><button class="button" onClick=${() => navigate('/filters')}>返回列表</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="inline-form" onSubmit=${submit}><label class="form-field">规则表达式<input class="input" placeholder="规则表达式" value=${rule} onInput=${e => setRule(e.currentTarget.value)} required /></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '添加中…' : '添加规则'}</button><button class="button" type="button" onClick=${() => navigate('/filters')}>取消</button></div></form></section>`;
}
