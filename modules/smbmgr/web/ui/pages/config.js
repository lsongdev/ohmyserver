import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useUnsavedChanges } from '../lib/forms.js';
import { savedToast, useApi } from '../samba.js';

export default function Config() {
  const file = useApi('/config');
  return html`<${Page} title="smb.conf" subtitle=${file.data?.path}>
    ${!file.data ? (file.error ? html`<${Notice} error=${file.error} />` : html`<${Loading} />`)
      : html`<${Editor} key=${file.data.version} file=${file.data} onSaved=${file.reload} />`}
  <//>`;
}

function Editor({ file, onSaved }) {
  const { api } = usePanel();
  const [text, setText] = useState(file.text);
  const [check, setCheck] = useState(null); // testparm's verdict on the text as checked
  const save = useAction();
  const test = useAction();
  const dirty = text !== file.text;
  const saved = useUnsavedChanges(dirty, { pending: save.busy });
  const edit = value => { setText(value); setCheck(null); };

  const runCheck = () => test.run(async () => setCheck(await api.post('/config/check', { text })));
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      try {
        const result = await api.put('/config', { text, version: file.version });
        savedToast('smb.conf saved', result);
        saved();
        onSaved();
      } catch (err) {
        // A rejected configuration: show testparm's verdict beside the text.
        if (err.code === 'invalid_config') setCheck({ ok: false, errors: err.message.split('\n'), warnings: [] });
        else throw err;
      }
    });
  };
  const busy = save.busy || test.busy;
  return html`<form class="form" onSubmit=${submit}>
    <${Notice} tone="info">Saving checks the file with testparm first and keeps the previous version as <code>${file.path}.bak</code>; smbd then reloads it.<//>
    <${Notice} error=${save.error || test.error} />
    ${check ? html`<${CheckResult} check=${check} />` : null}
    <textarea class="input code conf" spellcheck="false" aria-label="smb.conf" value=${text} disabled=${save.busy}
      onInput=${e => edit(e.currentTarget.value)} />
    <div class="form-actions">
      <button class="button button-primary" disabled=${busy || !dirty}>${save.busy ? 'Saving…' : 'Save'}</button>
      <button type="button" class="button" disabled=${busy} onClick=${runCheck}>${test.busy ? 'Checking…' : 'Check'}</button>
      <button type="button" class="button" disabled=${busy || !dirty} onClick=${() => { if (window.confirm('Discard your changes to smb.conf?')) edit(file.text); }}>Revert</button>
      ${dirty ? html`<span class="muted small">Unsaved changes</span>` : null}
    </div>
  </form>`;
}

function CheckResult({ check }) {
  if (check.ok && !check.warnings.length) return html`<${Notice} tone="success">testparm accepts this configuration.<//>`;
  return html`<${Notice} tone=${check.ok ? 'warn' : 'error'}>
    ${check.ok ? 'testparm accepts this configuration, with warnings:' : 'testparm rejects this configuration:'}
    <ul class="check">${[...check.errors, ...check.warnings].map(m => html`<li>${m}</li>`)}</ul>
  <//>`;
}
