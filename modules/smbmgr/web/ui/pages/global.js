import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { FormSection, useUnsavedChanges } from '../lib/forms.js';
import { canon, lookup, savedToast, useApi, ParamsEditor } from '../samba.js';

export default function Global() {
  const global = useApi('/global');
  return html`<${Page} title="Server settings" subtitle="The [global] section of smb.conf">
    ${!global.data ? (global.error ? html`<${Notice} error=${global.error} />` : html`<${Loading} />`)
      : html`<${GlobalForm} key=${global.data.version} global=${global.data} onSaved=${global.reload} />`}
  <//>`;
}

// The parameters the form has fields for, with their synonyms.
const FIELDS = [
  { key: 'workgroup', names: ['workgroup'], label: 'Workgroup', placeholder: 'WORKGROUP' },
  { key: 'server string', names: ['serverstring'], label: 'Description', placeholder: 'Samba %v', hint: 'Shown to clients next to the server' },
  { key: 'netbios name', names: ['netbiosname'], label: 'NetBIOS name', hint: 'Empty uses the host name' },
  { key: 'hosts allow', names: ['hostsallow', 'allowhosts'], label: 'Hosts allowed', mono: true, placeholder: '192.168.1.0/24 127.0.0.1', hint: 'Networks or addresses, separated by spaces; empty allows all' },
  { key: 'hosts deny', names: ['hostsdeny', 'denyhosts'], label: 'Hosts denied', mono: true, placeholder: '0.0.0.0/0', hint: 'Hosts allowed takes precedence over this' },
  { key: 'interfaces', names: ['interfaces'], label: 'Interfaces', mono: true, placeholder: 'eth0 192.168.1.10/24', hint: 'Empty listens on every interface' },
];
const SELECTS = [
  { key: 'map to guest', names: ['maptoguest'], label: 'Unknown users', options: [['', 'Default (never)'], ['never', 'Refuse'], ['bad user', 'Sign in as guest']] },
  { key: 'server min protocol', names: ['serverminprotocol', 'minprotocol'], label: 'Oldest protocol', options: [['', 'Default (SMB2_02)'], ['SMB2', 'SMB 2'], ['SMB3', 'SMB 3'], ['NT1', 'SMB 1 (insecure)']] },
];
const owned = new Set([...FIELDS, ...SELECTS].flatMap(f => f.names));

function fromParams(params) {
  const f = { other: params.filter(p => !owned.has(canon(p.key))).map(p => ({ ...p })) };
  for (const field of [...FIELDS, ...SELECTS]) f[field.key] = lookup(params, field.names)?.value ?? '';
  return f;
}

function toParams(f) {
  const out = [...FIELDS, ...SELECTS].filter(field => f[field.key].trim()).map(field => ({ key: field.key, value: f[field.key].trim() }));
  return [...out, ...f.other.filter(p => p.key.trim()).map(p => ({ key: p.key.trim(), value: p.value.trim() }))];
}

function GlobalForm({ global, onSaved }) {
  const { api } = usePanel();
  const [f, setF] = useState(() => fromParams(global.params));
  const [initial] = useState(() => JSON.stringify(f));
  const { busy, error, run } = useAction();
  const saved = useUnsavedChanges(JSON.stringify(f) !== initial, { pending: busy });
  const set = patch => setF(prev => ({ ...prev, ...patch }));
  const submit = e => {
    e.preventDefault();
    run(async () => {
      const result = await api.put('/global', { version: global.version, params: toParams(f) });
      savedToast('Server settings saved', result);
      saved();
      onSaved();
    });
  };
  const select = s => html`<${Field} label=${s.label} key=${s.key}>
    <select class="input" value=${f[s.key]} onChange=${e => set({ [s.key]: e.currentTarget.value })}>
      ${s.options.map(([v, l]) => html`<option value=${v}>${l}</option>`)}
      ${s.options.some(([v]) => v.toLowerCase() === f[s.key].toLowerCase()) ? null : html`<option value=${f[s.key]}>${f[s.key]}</option>`}
    </select>
  <//>`;
  const field = d => html`<${Field} label=${d.label} hint=${d.hint} key=${d.key}>
    <input class=${`input ${d.mono ? 'mono' : ''}`} value=${f[d.key]} placeholder=${d.placeholder} onInput=${e => set({ [d.key]: e.currentTarget.value })} />
  <//>`;
  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${busy}>
      <${Notice} error=${error} />
      <${FormSection} title="Identity">
        <div class="form-grid">${FIELDS.slice(0, 3).map(field)}</div>
      <//>
      <${FormSection} title="Network and access" description="Clients outside the allowed hosts cannot connect at all; this panel is not affected.">
        <div class="form-grid">${FIELDS.slice(3).map(field)}${SELECTS.map(select)}</div>
      <//>
      <${FormSection} title="Other parameters" description="Any other [global] parameter. testparm checks them when you save.">
        <${ParamsEditor} rows=${f.other} onChange=${other => set({ other })} disabled=${busy} />
      <//>
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${busy}>${busy ? 'Saving…' : 'Save changes'}</button>
    </div>
  </form>`;
}
