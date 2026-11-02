import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { FormSection, useUnsavedChanges } from '../lib/forms.js';
import { canon, lookup, savedToast, useApi, ParamsEditor } from '../samba.js';
import { ConfigText, CheckResult } from '../components/config-text.js';

export default function Global() {
  const file = useApi('/config');
  const [mode, setMode] = useState('form');
  if (!file.data) return html`<${Page} title="Server settings">${file.error ? html`<${Notice} error=${file.error} />` : html`<${Loading} />`}<//>`;
  return html`<${SettingsEditor} key=${file.data.version} file=${file.data} mode=${mode} setMode=${setMode} onSaved=${file.reload} />`;
}

// Common settings have named controls; unrecognized parameters stay in
// the custom editor. Synonyms are claimed here too so they cannot appear
// in both places or override a named field after saving.
const GROUPS = [
  { title: 'Identity', fields: [
    { key: 'workgroup', names: ['workgroup'], label: 'Workgroup', placeholder: 'WORKGROUP' },
    { key: 'server string', names: ['serverstring'], label: 'Description', placeholder: 'Samba %v', hint: 'Shown to clients next to the server' },
    { key: 'netbios name', names: ['netbiosname'], label: 'NetBIOS name', hint: 'Empty uses the host name' },
    { key: 'server role', names: ['serverrole'], label: 'Server role', options: [['', 'Default (auto)'], ['auto', 'Automatic'], ['standalone server', 'Standalone server'], ['member server', 'Domain member'], ['active directory domain controller', 'Active Directory domain controller']] },
  ] },
  { title: 'Network and access', description: 'These rules control connections to Samba, including connections through Tailscale. They do not affect this panel.', fields: [
    { key: 'hosts allow', names: ['hostsallow', 'allowhosts'], label: 'Hosts allowed', multiline: true, placeholder: '192.168.8.0/24\n127.0.0.1\n100.84.176.110', hint: 'One IP address or network range per line. Add each Tailscale client IP here. Empty adds no allow rule.' },
    { key: 'hosts deny', names: ['hostsdeny', 'denyhosts'], label: 'Hosts denied', multiline: true, placeholder: '0.0.0.0/0', hint: 'One IP address or network range per line. Hosts allowed takes precedence. Empty adds no deny rule.' },
    { key: 'interfaces', names: ['interfaces'], label: 'Interfaces', mono: true, placeholder: 'lo eth0 192.168.8.100/24', hint: 'Interface names or addresses, separated by spaces; empty uses the default interfaces' },
    { key: 'bind interfaces only', names: ['bindinterfacesonly'], label: 'Bind only to selected interfaces', hint: 'When enabled, include loopback (lo or 127.0.0.1) in Interfaces for local Samba tools.', options: [['', 'Default (no)'], ['no', 'No'], ['yes', 'Yes']] },
    { key: 'smb ports', names: ['smbports'], label: 'SMB ports', mono: true, placeholder: '445 139', hint: 'TCP ports, separated by spaces. Port 445 is used for direct SMB connections. Port changes require a Samba restart.' },
  ] },
  { title: 'Authentication and protocol', fields: [
    { key: 'security', names: ['security'], label: 'Security mode', hint: 'User mode authenticates clients with Samba accounts.', options: [['', 'Default (auto)'], ['auto', 'Automatic'], ['user', 'User accounts'], ['domain', 'Windows domain'], ['ads', 'Active Directory']] },
    { key: 'map to guest', names: ['maptoguest'], label: 'Unknown users', options: [['', 'Default (never)'], ['never', 'Refuse'], ['bad user', 'Sign in as guest'], ['bad password', 'Map incorrect passwords to guest'], ['bad uid', 'Map users without a Unix account to guest']] },
    { key: 'server min protocol', names: ['serverminprotocol', 'minprotocol'], label: 'Oldest protocol', options: [['', 'Default (SMB2_02)'], ['SMB2', 'SMB 2'], ['SMB3', 'SMB 3'], ['NT1', 'SMB 1 (insecure)']] },
    { key: 'server max protocol', names: ['servermaxprotocol', 'maxprotocol'], label: 'Newest protocol', options: [['', 'Default (SMB3)'], ['SMB2', 'SMB 2'], ['SMB3', 'SMB 3']] },
  ] },
  { title: 'Logging', description: 'Control where Samba writes logs and how much detail it records.', fields: [
    { key: 'log file', names: ['logfile'], label: 'Log file', mono: true, placeholder: '/var/log/samba/%m.log', hint: 'Absolute path on this server; %m creates a separate log for each client name.' },
    { key: 'logging', names: ['logging'], label: 'Logging backend', mono: true, placeholder: 'file', hint: 'For example file, systemd, or syslog. Multiple backends can be separated by spaces.' },
    { key: 'log level', names: ['loglevel', 'debuglevel'], label: 'Log level', mono: true, placeholder: '0', hint: '0 logs critical events. Higher values add detail; class levels such as auth:3 are also supported.' },
    { key: 'max log size', names: ['maxlogsize'], label: 'Maximum log size (KB)', type: 'number', min: '0', step: '1', placeholder: '5000', hint: 'Rotate each log after this size. Set 0 for no size limit.' },
  ] },
];
const FIELDS = GROUPS.flatMap(group => group.fields);
const owned = new Set(FIELDS.flatMap(field => field.names));

function fromParams(params) {
  const f = { other: params.filter(p => !owned.has(canon(p.key))).map(p => ({ ...p })) };
  for (const field of FIELDS) {
    const value = lookup(params, field.names)?.value ?? '';
    f[field.key] = field.multiline ? value.trim().replace(/[\s,]+/g, '\n') : value;
  }
  return f;
}

function toParams(f) {
  // smb.conf parameters occupy a single line, even when edited as a list.
  const out = FIELDS.map(field => ({ key: field.key, value: field.multiline ? f[field.key].trim().replace(/[\s,]+/g, ' ') : f[field.key].trim() })).filter(p => p.value);
  return [...out, ...f.other.filter(p => p.key.trim()).map(p => ({ key: p.key.trim(), value: p.value.trim() }))];
}

function SettingsEditor({ file, mode, setMode, onSaved }) {
  const { api } = usePanel();
  const [f, setF] = useState(() => fromParams(file.params));
  const [syncedForm, setSyncedForm] = useState(() => JSON.stringify(f));
  const [text, setText] = useState(file.text);
  const [check, setCheck] = useState(null);
  const action = useAction();
  const busy = action.busy;
  const formChanged = JSON.stringify(f) !== syncedForm;
  const dirty = text !== file.text || (mode === 'form' && formChanged);
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const set = patch => setF(prev => ({ ...prev, ...patch }));
  const prepare = async () => {
    if (mode !== 'form' || !formChanged) return text;
    const draft = await api.post('/config/preview', { text, params: toParams(f) });
    setText(draft.text);
    setSyncedForm(JSON.stringify(f));
    return draft.text;
  };
  const switchMode = next => {
    if (next === mode) return;
    action.run(async () => {
      if (next === 'raw') await prepare();
      else {
        const draft = await api.post('/config/preview', { text });
        const form = fromParams(draft.params);
        setF(form);
        setSyncedForm(JSON.stringify(form));
      }
      setCheck(null);
      setMode(next);
    });
  };
  const runCheck = () => action.run(async () => setCheck(await api.post('/config/check', { text: await prepare() })));
  const submit = e => {
    e.preventDefault();
    action.run(async () => {
      try {
        const result = await api.put('/config', { version: file.version, text: await prepare() });
        savedToast('Server settings saved', result);
        saved();
        onSaved();
      } catch (err) {
        if (err.code === 'invalid_config') setCheck({ ok: false, errors: err.message.split('\n'), warnings: [] });
        else throw err;
      }
    });
  };
  const revert = () => {
    if (!window.confirm('Discard your unsaved settings changes?')) return;
    const form = fromParams(file.params);
    setF(form);
    setSyncedForm(JSON.stringify(form));
    setText(file.text);
    setCheck(null);
    action.setError(null);
  };

  return html`<${Page} title="Server settings" subtitle=${file.path} actions=${html`<span class="samba-editor-mode" aria-label="Editor mode">
        <button type="button" class=${`button ${mode === 'form' ? 'active' : ''}`} aria-pressed=${mode === 'form'} disabled=${busy} onClick=${() => switchMode('form')}>Form</button>
        <button type="button" class=${`button ${mode === 'raw' ? 'active' : ''}`} aria-pressed=${mode === 'raw'} disabled=${busy} onClick=${() => switchMode('raw')}>smb.conf</button>
    </span>`}>
    <form class=${`form ${mode === 'form' ? 'settings-form' : ''}`} onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${busy}>
        <${Notice} error=${action.error} />
        ${check ? html`<${CheckResult} check=${check} />` : null}
        ${mode === 'form' ? html`<${GlobalFields} f=${f} set=${patch => { set(patch); setCheck(null); }} busy=${busy} />`
          : html`<${ConfigText} text=${text} path=${file.path} onChange=${value => { setText(value); setCheck(null); }} />`}
      </fieldset>
      <div class="button-actions">
        <button class="button button-primary" disabled=${busy || !dirty}>${busy ? 'Working…' : 'Save'}</button>
        <button type="button" class="button" disabled=${busy} onClick=${runCheck}>Check</button>
        <button type="button" class="button" disabled=${busy || !dirty} onClick=${revert}>Revert</button>
        ${dirty ? html`<span class="muted small">Unsaved changes</span>` : null}
      </div>
    </form>
  <//>`;
}

function GlobalFields({ f, set, busy }) {
  const field = d => html`<${Field} label=${d.label} hint=${d.hint} key=${d.key}>
    ${d.options ? html`<select class="input" value=${f[d.key]} onChange=${e => set({ [d.key]: e.currentTarget.value })}>
      ${d.options.map(([v, l]) => html`<option value=${v}>${l}</option>`)}
      ${d.options.some(([v]) => v === f[d.key]) ? null : html`<option value=${f[d.key]}>${f[d.key]}</option>`}
    </select>` : d.multiline ? html`<textarea class="input mono" rows="5" spellcheck="false" autocapitalize="off" value=${f[d.key]}
      placeholder=${d.placeholder} onInput=${e => set({ [d.key]: e.currentTarget.value })}></textarea>`
      : html`<input class=${`input ${d.mono ? 'mono' : ''}`} type=${d.type || 'text'} min=${d.min} step=${d.step}
          value=${f[d.key]} placeholder=${d.placeholder} onInput=${e => set({ [d.key]: e.currentTarget.value })} />`}
  <//>`;
  return html`
      ${GROUPS.map(group => html`<${FormSection} key=${group.title} title=${group.title} description=${group.description}>
        <div class="form-grid">${group.fields.map(field)}</div>
      <//>`)}
      <${FormSection} title="Custom parameters" description="Additional [global] settings without a dedicated field above. Enter their Samba parameter names and values; testparm validates all settings when you save.">
        <${ParamsEditor} rows=${f.other} onChange=${other => set({ other })} disabled=${busy} />
      <//>
  `;
}
