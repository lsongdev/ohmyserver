import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, Loading, Empty, Table, Badge, Field } from '@lsongdev/scripts/react/components/index.js';
import { Page, useApi } from '../network.js';

const loopback = a => a.startsWith('127.') || a === '::1' || a.endsWith('%lo');
const everywhere = a => a === '0.0.0.0' || a === '::' || a === '*';

export default function Ports({ query, setQuery }) {
  const { api } = usePanel();
  const ports = useApi('/ports', 10000);
  const action = useAction();
  const q = (query.q || '').toLowerCase();
  const shown = (ports.data || []).filter(p => !q || `${p.protocol} ${p.address} ${p.port} ${p.process || ''}`.toLowerCase().includes(q));
  return html`<${Page} title="Ports" subtitle="What this machine listens on. Refreshes every few seconds.">
    <${Alert} error=${ports.error || action.error} />
    <div class="filters">
      <${Field} label="Filter">
        <input class="input" type="search" value=${query.q || ''} placeholder="Port, address or process" onInput=${e => setQuery({ q: e.currentTarget.value })} />
      <//>
    </div>
    ${!ports.data ? (ports.error ? null : html`<${Loading} />`) : shown.length === 0 ? html`<${Empty}>Nothing listening matches.<//>` : html`<${Table}>
      <thead><tr><th>Port</th><th>Protocol</th><th>Address</th><th>Reachable from</th><th>Process</th><th class="actions">Action</th></tr></thead>
      <tbody>${shown.map((p, i) => html`<tr key=${i}>
        <td class="cell-main">${p.port}</td>
        <td>${p.protocol.toUpperCase()}</td>
        <td class="mono">${p.address}</td>
        <td>${loopback(p.address) ? html`<${Badge}>This machine<//>` : everywhere(p.address) ? html`<${Badge} variant="warning">All networks<//>` : html`<${Badge} variant="info">That address<//>`}</td>
        <td>${p.process ? html`${p.process} <span class="muted small">${p.pid}</span>` : html`<span class="muted">—</span>`}</td>
        <td class="actions">${p.pid > 1 ? html`<button class="button button-small button-danger" disabled=${action.busy}
          onClick=${() => {
            if (!window.confirm(`Send SIGTERM to ${p.process || 'process'} (${p.pid}) listening on ${p.port}/${p.protocol}? This may stop the service.`)) return;
            action.run(async () => { await api.post(`/ports/${p.pid}/kill`); setTimeout(ports.reload, 500); });
          }}>Kill</button>` : html`<span class="muted">—</span>`}</td>
      </tr>`)}</tbody>
    <//>`}
  <//>`;
}
