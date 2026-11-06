import { html } from '@lsongdev/panel/html.js';
import { Notice, Loading, Empty, Table, Badge, Field } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi } from '../network.js';

export default function Routes({ query, setQuery }) {
  const { href } = usePanel();
  const routes = useApi('/routes', 10000);
  const family = query.family || '';
  const device = query.device || '';
  const devices = [...new Set((routes.data || []).map(r => r.device).filter(Boolean))].sort();
  const shown = (routes.data || []).filter(r => (!family || r.family === family) && (!device || r.device === device));
  return html`<${Page} title="Routes" subtitle="Where this machine sends traffic, from the main routing table. Permanent routes are set in an interface's settings.">
    <${Notice} error=${routes.error} />
    <div class="filters">
      <${Field} label="Family">
        <select class="input" value=${family} onChange=${e => setQuery({ family: e.currentTarget.value })}>
          <option value="">All</option><option value="inet">IPv4</option><option value="inet6">IPv6</option>
        </select>
      <//>
      <${Field} label="Interface">
        <select class="input" value=${device} onChange=${e => setQuery({ device: e.currentTarget.value })}>
          <option value="">All</option>
          ${devices.map(d => html`<option key=${d} value=${d}>${d}</option>`)}
        </select>
      <//>
    </div>
    ${!routes.data ? (routes.error ? null : html`<${Loading} />`) : shown.length === 0 ? html`<${Empty}>No routes match.<//>` : html`<${Table}>
      <thead><tr><th>Destination</th><th>Via</th><th>Interface</th><th>Added by</th><th>Metric</th></tr></thead>
      <tbody>${shown.map((r, i) => html`<tr key=${i}>
        <td class="mono">${r.destination}${r.destination === 'default' ? html` <${Badge} tone="blue">${r.family === 'inet6' ? 'IPv6' : 'IPv4'}<//>` : null}${r.type && r.type !== 'unicast' ? html` <${Badge}>${r.type}<//>` : null}</td>
        <td class="mono">${r.gateway || html`<span class="muted">direct</span>`}</td>
        <td class="mono">${r.device ? html`<a href=${href(`/interfaces/${encodeURIComponent(r.device)}`)}>${r.device}</a>` : '—'}</td>
        <td>${r.protocol || '—'}</td>
        <td>${r.metric || '—'}</td>
      </tr>`)}</tbody>
    <//>`}
  <//>`;
}
