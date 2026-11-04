import { html } from '../lib/html.js';
import { Notice, Loading, Empty, Table, Badge, Field } from '../lib/ui.js';
import { Page, useApi } from '../network.js';

const loopback = a => a.startsWith('127.') || a === '::1' || a.endsWith('%lo');
const everywhere = a => a === '0.0.0.0' || a === '::' || a === '*';

export default function Ports({ query, setQuery }) {
  const ports = useApi('/ports', 10000);
  const q = (query.q || '').toLowerCase();
  const shown = (ports.data || []).filter(p => !q || `${p.protocol} ${p.address} ${p.port} ${p.process || ''}`.toLowerCase().includes(q));
  return html`<${Page} title="Ports" subtitle="What this machine listens on. Refreshes every few seconds.">
    <${Notice} error=${ports.error} />
    <div class="filters">
      <${Field} label="Filter">
        <input class="input" type="search" value=${query.q || ''} placeholder="Port, address or process" onInput=${e => setQuery({ q: e.currentTarget.value })} />
      <//>
    </div>
    ${!ports.data ? (ports.error ? null : html`<${Loading} />`) : shown.length === 0 ? html`<${Empty}>Nothing listening matches.<//>` : html`<${Table}>
      <thead><tr><th>Port</th><th>Protocol</th><th>Address</th><th>Reachable from</th><th>Process</th></tr></thead>
      <tbody>${shown.map((p, i) => html`<tr key=${i}>
        <td class="cell-main">${p.port}</td>
        <td>${p.protocol.toUpperCase()}</td>
        <td class="mono">${p.address}</td>
        <td>${loopback(p.address) ? html`<${Badge}>This machine<//>` : everywhere(p.address) ? html`<${Badge} tone="amber">All networks<//>` : html`<${Badge} tone="blue">That address<//>`}</td>
        <td>${p.process ? html`${p.process} <span class="muted small">${p.pid}</span>` : html`<span class="muted">—</span>`}</td>
      </tr>`)}</tbody>
    <//>`}
  <//>`;
}
