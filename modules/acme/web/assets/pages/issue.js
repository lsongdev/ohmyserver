import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function IssueCertificate({ api, navigate }) {
  const [accounts, setAccounts] = useState([]);
  const [targets, setTargets] = useState([]);
  const [domains, setDomains] = useState('');
  const [accountID, setAccountID] = useState('');
  const [challengeType, setChallengeType] = useState('http-01');
  const [dnsTarget, setDNSTarget] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([api.listAccounts({ signal: controller.signal }), api.listChallengeTargets({ signal: controller.signal })])
      .then(([accountList, dns]) => { setAccounts(accountList || []); setTargets(dns?.targets || []); })
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '初始化签发选项失败'); });
    return () => controller.abort();
  }, [api]);
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try {
      const issuance = await api.startIssuance({ domains, account_id: Number(accountID), challenge_type: challengeType, dns_target: challengeType === 'dns-01' ? dnsTarget : '' });
      navigate(`/acme/issuances/${issuance.id}`);
    } catch (err) { setError(err.message || '无法开始证书签发'); } finally { setBusy(false); }
  };
  return html`<div><h2>申请证书</h2>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <form class="section" onSubmit=${submit}>
      <div class="form-group"><label for="domains">域名</label><textarea class="input" id="domains" value=${domains} onInput=${e => setDomains(e.currentTarget.value)} placeholder="example.com, www.example.com" required rows="3"></textarea><small>可输入多个域名，以逗号或空格分隔；通配符需使用 DNS-01。</small></div>
      <div class="form-group"><label for="account">ACME 账户</label><select class="input" id="account" value=${accountID} onChange=${e => setAccountID(e.currentTarget.value)} required><option value="">选择有效账户</option>${accounts.filter(item => item.status === 'valid').map(item => html`<option value=${item.id}>${item.provider_name} · ${item.email}</option>`)}</select></div>
      <div class="form-group"><label for="challenge">验证方式</label><select class="input" id="challenge" value=${challengeType} onChange=${e => setChallengeType(e.currentTarget.value)}><option value="http-01">HTTP-01</option><option value="dns-01">DNS-01（支持通配符）</option></select></div>
      ${challengeType === 'dns-01' ? html`<div class="form-group"><label for="dns-target">DNS 区域</label><select class="input" id="dns-target" value=${dnsTarget} onChange=${e => setDNSTarget(e.currentTarget.value)} required><option value="">选择 DNS 区域</option>${targets.map(target => html`<option value=${target.value}>${target.label} (${target.zone_name})</option>`)}</select></div>` : null}
      <div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy || !accounts.some(item => item.status === 'valid')}>${busy ? '正在提交…' : '开始签发'}</button><button class="button" type="button" onclick=${() => navigate('/acme/certificates')}>取消</button></div>
    </form>
  </div>`;
}
