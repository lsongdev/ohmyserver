import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function Certificate({ api, params, navigate }) {
  const [item, setItem] = useState(null);
  const [accounts, setAccounts] = useState([]);
  const [targets, setTargets] = useState([]);
  const [challengeType, setChallengeType] = useState('http-01');
  const [accountID, setAccountID] = useState('');
  const [dnsTarget, setDNSTarget] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    api.getCertificate(params.id, { signal: controller.signal }).then(setItem).catch(err => { if (err.name !== 'AbortError') setError(err.message || '证书读取失败'); });
    api.listAccounts({ signal: controller.signal }).then(value => { setAccounts(value); const account = value.find(entry => entry.status === 'valid'); if (account) setAccountID(String(account.id)); }).catch(err => { if (err.name !== 'AbortError') setError(err.message || '账户读取失败'); });
    return () => controller.abort();
  }, [api, params.id]);
  const wildcard = item?.domains?.some(domain => domain.startsWith('*.'));
  useEffect(() => {
    if (challengeType !== 'dns-01' && !wildcard) return undefined;
    const controller = new AbortController();
    api.listChallengeTargets({ signal: controller.signal }).then(value => setTargets(value.targets || [])).catch(err => { if (err.name !== 'AbortError') setError(err.message || 'DNS 验证目标读取失败'); });
    return () => controller.abort();
  }, [api, challengeType, wildcard]);
  const renew = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try {
      const selectedChallengeType = wildcard ? 'dns-01' : challengeType;
      const result = await api.renewCertificate(item.id, { account_id: Number(accountID), challenge_type: selectedChallengeType, dns_target: selectedChallengeType === 'dns-01' ? dnsTarget : '' });
      navigate(`/issuances/${result.id}`);
    } catch (err) { setError(err.message || '续签请求失败'); }
    finally { setBusy(false); }
  };
  const eligibleAccounts = accounts.filter(account => account.status === 'valid');
  return html`<div><div class="page-toolbar"><h2>证书详情</h2><button class="button" onclick=${() => navigate('/certificates')}>返回证书</button></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    ${item ? html`<section class="section"><dl><dt>ID</dt><dd>${item.id}</dd><dt>域名</dt><dd>${item.domains.join(', ')}</dd><dt>状态</dt><dd>${item.status}</dd><dt>到期时间</dt><dd>${item.expires_at ? new Date(item.expires_at).toLocaleString() : '未知'}</dd></dl>
      ${item.cert_pem ? html`<h3>证书 PEM</h3><textarea class="input mono" readonly rows="12" value=${item.cert_pem}></textarea>` : html`<p>证书尚未签发。</p>`}
      ${item.csr_pem ? html`<h3>CSR</h3><textarea class="input mono" readonly rows="12" value=${item.csr_pem}></textarea>` : null}
      <div class="form-actions">${item.cert_pem ? html`<a class="button button-primary" href=${api.certificateDownloadURL(item.id)}>下载证书与私钥 ZIP</a>` : null}</div>
      <p class="muted">私钥不会通过详情 API 返回。</p>
    </section>
    ${item.status === 'active' ? html`<section class="section"><h3>续签证书</h3><p>续签会保留当前证书，直到新证书签发成功。</p><form onSubmit=${renew}>
      <div class="form-group"><label>ACME 账户</label><select class="input" required value=${accountID} onChange=${event => setAccountID(event.currentTarget.value)}><option value="">选择账户</option>${eligibleAccounts.map(account => html`<option value=${account.id}>${account.email} · ${account.provider_name}</option>`)}</select></div>
      <div class="form-group"><label>验证方式</label><select class="input" value=${wildcard ? 'dns-01' : challengeType} disabled=${wildcard} onChange=${event => setChallengeType(event.currentTarget.value)}><option value="http-01">HTTP-01</option><option value="dns-01">DNS-01</option></select></div>
      ${challengeType === 'dns-01' || wildcard ? html`<div class="form-group"><label>DNS 区域</label><select class="input" required value=${dnsTarget} onChange=${event => setDNSTarget(event.currentTarget.value)}><option value="">选择区域</option>${targets.map(target => html`<option value=${target.value}>${target.label}</option>`)}</select></div>` : null}
      <div class="form-actions"><button class="button button-primary" disabled=${busy || !accountID || (wildcard && !dnsTarget)}>${busy ? '提交中…' : '开始续签'}</button></div>
    </form></section>` : null}` : !error ? html`<div class="loading-state">正在读取…</div>` : null}</div>`;
}
