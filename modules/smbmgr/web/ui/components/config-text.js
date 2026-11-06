import { html } from '@lsongdev/panel/html.js';
import { Notice } from '@lsongdev/panel/ui.js';

export function ConfigText({ text, onChange, path }) {
  return html`
    <${Notice} tone="info">Edit the complete configuration, including shares. Saving checks it with testparm and keeps the previous version as <code>${path}.bak</code>.<//>
    <textarea class="input code conf" spellcheck="false" aria-label="smb.conf" value=${text}
      onInput=${e => onChange(e.currentTarget.value)} />
  `;
}

export function CheckResult({ check }) {
  if (check.ok && !check.warnings.length) return html`<${Notice} tone="success">testparm accepts this configuration.<//>`;
  return html`<${Notice} tone=${check.ok ? 'warn' : 'error'}>
    ${check.ok ? 'testparm accepts this configuration, with warnings:' : 'testparm rejects this configuration:'}
    <ul class="check">${[...check.errors, ...check.warnings].map(m => html`<li>${m}</li>`)}</ul>
  <//>`;
}
