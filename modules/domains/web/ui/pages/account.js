import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi } from '../domains.js';

// Account adds an account (/accounts/new) or edits one (/accounts/:id).
export default function Account({ params }) {
  const { href } = usePanel();
  const isNew = !params.id;
  const kinds = useApi('/kinds');
  const account = useApi(isNew ? null : `/accounts/${params.id}`);
  const title = isNew ? 'Add account' : account.data ? account.data.name : 'Account';
  const back = { href: href('/accounts'), label: 'Accounts' };
  if (!kinds.data || (!isNew && !account.data)) {
    return html`<${PageHeader} title=${title} back=${back} />${kinds.error || account.error
      ? html`<${Alert} error=${kinds.error || account.error} />` : html`<${Loading} />`}`;
  }
  return html`<${PageHeader} title=${title} back=${back} /><${AccountForm} kinds=${kinds.data} account=${account.data} />`;
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
      navigate('/accounts');
    });
  };
  const runTest = () => test.run(async () => setZones(await api.post('/accounts/test', { id: account?.id || 0, ...body() })));
  return html`<form class="form form-narrow" onSubmit=${submit}>
    ${isNew ? html`<${Field} label="Provider">
      <select class="input" value=${kind} onChange=${e => { setKind(e.currentTarget.value); setCreds({}); }}>
        ${kinds.map(x => html`<option key=${x.id} value=${x.id}>${x.title}</option>`)}
      </select>
    <//>` : null}
    ${k?.help ? html`<${Alert} variant="info">${k.help}<//>` : null}
    <${Field} label="Name" hint="To tell accounts apart">
      <input class="input" value=${name} placeholder=${k?.title} onInput=${e => setName(e.currentTarget.value)} />
    <//>
    ${(k?.fields || []).map(f => html`<${Field} key=${f.name} label=${f.label} required=${!f.optional && (isNew || !account.set?.[f.name])}
      hint=${!isNew && account.set?.[f.name] ? `Leave empty to keep the saved ${f.label.toLowerCase()}` : f.hint}>
      <input class="input mono" type=${f.secret ? 'password' : 'text'} autocomplete="off" required=${!f.optional && (isNew || !account.set?.[f.name])}
        value=${creds[f.name] || ''} onInput=${e => { setCreds({ ...creds, [f.name]: e.currentTarget.value }); setZones(null); }} />
    <//>`)}
    <${Alert} error=${save.error || test.error} />
    ${zones ? html`<${Alert} variant="success">Connected. ${zones.length
      ? html`It reaches ${zones.length} ${zones.length === 1 ? 'domain' : 'domains'}: ${zones.map(z => z.name).join(', ')}.`
      : 'It reaches no domains yet: check the account and its domain permissions.'}<//>` : null}
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || test.busy}>${save.busy ? 'Checking…' : isNew ? 'Add account' : 'Save'}</button>
      <button class="button" type="button" disabled=${test.busy || save.busy} onClick=${runTest}>${test.busy ? 'Testing…' : 'Test connection'}</button>
    </div>
  </form>`;
}
