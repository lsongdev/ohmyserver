import { html } from '@module/ui';
import { useState } from 'preact/hooks';

export default function CreateCSR({ api, navigate }) {
  const [domains, setDomains] = useState('');
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); setResult(null); try { setResult(await api.generateCSR({ domains })); } catch (err) { setError(err.message || 'CSR 生成失败'); } finally { setBusy(false); } };
  return html`<div><div class="page-toolbar"><h2>生成证书签名请求</h2><button class="button" onclick=${() => navigate('/certificates')}>返回证书</button></div>${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}<form class="section" onSubmit=${submit}><div class="form-group"><label for="csr-domains">域名</label><textarea class="input" id="csr-domains" value=${domains} onInput=${e => setDomains(e.currentTarget.value)} placeholder="example.com, www.example.com" required rows="3"></textarea></div><button class="button button-primary" type="submit" disabled=${busy}>${busy ? '生成中…' : '生成 CSR'}</button></form>${result ? html`<section class="section"><h3>CSR 已保存 (#${result.id})</h3><p>${result.domains.join(', ')}</p><textarea class="input mono" readonly rows="12" value=${result.csr_pem}></textarea></section>` : null}</div>`;
}
