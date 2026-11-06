import { html } from '@lsongdev/panel/html.js';
import { Alert } from '@lsongdev/scripts/react/components/index.js';

export function ConfigText({ text, onChange, path }) {
  return html`
    <${Alert} variant="info">Edit the complete configuration, including shares. Saving checks it with testparm and keeps the previous version as <code>${path}.bak</code>.<//>
    <textarea class="input code conf" spellcheck="false" aria-label="smb.conf" value=${text}
      onInput=${e => onChange(e.currentTarget.value)} />
  `;
}

export function CheckResult({ check }) {
  if (check.ok && !check.warnings.length) return html`<${Alert} variant="success">testparm accepts this configuration.<//>`;
  return html`<${Alert} variant=${check.ok ? 'warning' : 'danger'}>
    ${check.ok ? 'testparm accepts this configuration, with warnings:' : 'testparm rejects this configuration:'}
    <ul class="check">${[...check.errors, ...check.warnings].map(m => html`<li>${m}</li>`)}</ul>
  <//>`;
}
