import { html, useEffect, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi } from '../domains.js';

// Account adds an account (/accounts/new) or edits one (/accounts/:id).
export default function Account({ params }) {
  const { href } = usePanel();
  const isNew = !params.id;
  const kinds = useApi('/kinds');
  const account = useApi(isNew ? null : `/accounts/${params.id}`);
  const title = isNew ? 'Add account' : account.data ? account.data.name : 'Account';
  const back = html`<a href=${href('/accounts')}>← Accounts</a>`;
  if (!kinds.data || (!isNew && !account.data)) {
    return html`<${Page} title=${title} subtitle=${back}>${kinds.error || account.error
      ? html`<${Notice} error=${kinds.error || account.error} />` : html`<${Loading} />`}<//>`;
  }
  return html`<${Page} title=${title} subtitle=${back}><${AccountForm} kinds=${kinds.data} account=${account.data} /><//>`;
}

function AccountForm({ kinds, account }) {
  const { api, navigate } = usePanel();
  const isNew = !account;
  const [kind, setKind] = useState(account?.kind || kinds[0]?.id || '');
  const [name, setName] = useState(account?.name || '');
  const [creds, setCreds] = useState({});
  const [zones, setZones] = useState(null); // what a test reached
  const save = useAction();
  const test = useAction();
  const k = kinds.find(x => x.id === kind);
  useEffect(() => { setZones(null); }, [kind]);
  const body = () => ({ name: name.trim(), kind, credentials: creds });
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      if (isNew) await api.post('/accounts', body());
      else await api.put(`/accounts/${account.id}`, body());
      toast(isNew ? 'Account added' : 'Account saved', 'success');
      navigate('/');
    });
  };
  const runTest = () => test.run(async () => setZones(await api.post('/accounts/test', { id: account?.id || 0, ...body() })));
  return html`<form class="form form-narrow" onSubmit=${submit}>
    ${isNew ? html`<${Field} label="Provider">
      <div class="choice-grid">${kinds.map(x => html`<label key=${x.id} class=${`choice-card ${kind === x.id ? 'selected' : ''}`}>
        <input type="radio" name="kind" checked=${kind === x.id} onChange=${() => setKind(x.id)} /><b>${x.title}</b></label>`)}</div>
    <//>` : null}
    ${k?.help ? html`<${Notice} tone="info">${k.help}<//>` : null}
    <${Field} label="Name" hint="To tell accounts apart">
      <input class="input" value=${name} placeholder=${k?.title} onInput=${e => setName(e.currentTarget.value)} />
    <//>
    ${(k?.fields || []).map(f => html`<${Field} key=${f.name} label=${f.label} required=${isNew || !account.set?.[f.name]}
      hint=${!isNew && account.set?.[f.name] ? `Leave empty to keep the saved ${f.label.toLowerCase()}` : f.hint}>
      <input class="input mono" type=${f.secret ? 'password' : 'text'} autocomplete="off" required=${isNew || !account.set?.[f.name]}
        value=${creds[f.name] || ''} onInput=${e => { setCreds({ ...creds, [f.name]: e.currentTarget.value }); setZones(null); }} />
    <//>`)}
    <${Notice} error=${save.error || test.error} />
    ${zones ? html`<${Notice} tone="success">Connected. ${zones.length
      ? html`It reaches ${zones.length} ${zones.length === 1 ? 'domain' : 'domains'}: ${zones.map(z => z.name).join(', ')}.`
      : 'It reaches no domains yet: check the token\'s zone permissions.'}<//>` : null}
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy}>${save.busy ? 'Checking…' : isNew ? 'Add account' : 'Save'}</button>
      <button class="button" type="button" disabled=${test.busy} onClick=${runTest}>${test.busy ? 'Testing…' : 'Test connection'}</button>
    </div>
  </form>`;
}
