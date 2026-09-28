import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function RegisterAccount({ api, params, navigate }) {
  const [provider, setProvider] = useState(null);
  const [email, setEmail] = useState('');
  const [agree, setAgree] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => { const controller = new AbortController(); api.listProviders({ signal: controller.signal }).then(items => setProvider(items.find(item => String(item.id) === String(params.id)) || null)).catch(err => { if (err.name !== 'AbortError') setError(err.message); }); return () => controller.abort(); }, [api, params.id]);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.registerAccount(params.id, { email, agreed_terms: agree }); navigate('/accounts'); } catch (err) { setError(err.message || '账户注册失败'); } finally { setBusy(false); } };
  return html`<div><div class="page-toolbar"><h2>注册 ACME 账户</h2><button class="button" onclick=${() => navigate('/providers')}>返回服务商</button></div>${provider ? html`<p>${provider.name} · ${provider.directory_url}</p>` : null}${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}<form class="section acme-register-form" onSubmit=${submit}><div class="form-group"><label for="email">联系邮箱</label><input class="input" id="email" type="email" value=${email} onInput=${e => setEmail(e.currentTarget.value)} required /></div><div class="form-group acme-terms"><label class="acme-terms-choice"><input class="input" type="checkbox" checked=${agree} onChange=${e => setAgree(e.currentTarget.checked)} /> <span>我已阅读并同意</span></label>${provider?.terms_of_service ? html`<a href=${provider.terms_of_service} target="_blank" rel="noreferrer">服务条款</a>` : html`<span>服务条款</span>`}</div><div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy || !provider}>${busy ? '注册中…' : '注册'}</button></div></form></div>`;
}
