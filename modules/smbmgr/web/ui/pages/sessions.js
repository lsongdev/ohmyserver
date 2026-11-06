import { html } from '@lsongdev/panel/html.js';
import { PageHeader, Alert, Loading, Empty, Table, Badge } from '@lsongdev/scripts/react/components/index.js';
import { datetime, ago } from '@lsongdev/panel/format.js';
import { useApi } from '../samba.js';

const since = t => html`<span title=${datetime(t)}>${ago(t)}</span>`;
const SHOW_FILES = 200;

export default function Sessions() {
  const status = useApi('/status', 5000);
  const s = status.data;
  return html`<${PageHeader} title="Sessions" subtitle="Who is connected, as smbstatus reports it; refreshes every few seconds" />
    <${Alert} error=${status.error} />
    ${!s ? (status.error ? null : html`<${Loading} />`) : html`
      <section>
        <h2 class="section-title">Clients (${s.sessions.length})</h2>
        ${s.sessions.length === 0 ? html`<${Empty}>No one is connected.<//>` : html`<${Table}>
          <thead><tr><th>User</th><th>Client</th><th>Protocol</th><th>Security</th><th>Since</th></tr></thead>
          <tbody>${s.sessions.map(x => html`<tr key=${x.id}>
            <td><span class="cell-main">${x.user}</span>${x.group && x.group !== x.user ? html`<div class="cell-sub">${x.group}</div>` : null}</td>
            <td class="mono">${x.machine}</td>
            <td>${x.dialect}</td>
            <td><span class="tags">
              ${x.encryption && x.encryption !== '-' ? html`<${Badge} variant="success">Encrypted<//>` : html`<${Badge}>Not encrypted<//>`}
              ${x.signing && x.signing !== '-' ? html`<${Badge} variant="info" title=${x.signing}>Signed<//>` : null}
            </span></td>
            <td>${since(x.since)}</td>
          </tr>`)}</tbody>
        <//>`}
      </section>
      <section class="section">
        <h2 class="section-title">Shares in use (${s.connections.length})</h2>
        ${s.connections.length === 0 ? html`<${Empty}>No share is in use.<//>` : html`<${Table}>
          <thead><tr><th>Share</th><th>Client</th><th>Since</th></tr></thead>
          <tbody>${s.connections.map((c, i) => html`<tr key=${i}>
            <td class="cell-main">${c.share}</td><td class="mono">${c.machine}</td><td>${since(c.since)}</td>
          </tr>`)}</tbody>
        <//>`}
      </section>
      <section class="section">
        <h2 class="section-title">Open files (${s.open_files.length})</h2>
        ${s.open_files.length === 0 ? html`<${Empty}>No files are open.<//>` : html`<${Table}>
          <thead><tr><th>File</th><th>Access</th><th>Since</th></tr></thead>
          <tbody>${s.open_files.slice(0, SHOW_FILES).map(f => html`<tr key=${f.path}>
            <td><span class="cell-main mono">${f.name}</span><div class="cell-sub">${f.share}</div></td>
            <td>${f.access}${f.opens > 1 ? html` <span class="muted">(${f.opens} handles)</span>` : null}</td>
            <td>${since(f.since)}</td>
          </tr>`)}</tbody>
        <//>`}
        ${s.open_files.length > SHOW_FILES ? html`<p class="muted small">Showing ${SHOW_FILES} of ${s.open_files.length}.</p>` : null}
      </section>`}`;
}
