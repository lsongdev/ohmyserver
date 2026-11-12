import { html } from '@lsongdev/panel/html.js';
import { Alert, Loading, Empty, Table, Badge } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi, useChanged } from '../network.js';

// Bridges join interfaces into one network. Making one is a page of its own
// (see bridge.js); this lists them.
export default function Bridges() {
  const { href } = usePanel();
  const info = useApi('/info');
  const supported = info.data?.backend === 'NetworkManager';
  const bridges = useApi(supported ? '/bridges' : null, 5000);
  useChanged(bridges.reload);
  const actions = supported ? html`<a class="button button-primary" href=${href('/bridges/new')}>New bridge</a>` : null;
  return html`<${Page} title="Bridges" subtitle="Join interfaces into one network" actions=${actions}>
    <${Alert} error=${info.error || bridges.error} />
    ${!info.data ? (info.error ? null : html`<${Loading} />`)
      : !supported ? html`<${Alert} variant="info">Bridges are made through NetworkManager, which is not what runs on this machine${info.data.backend ? ` (${info.data.backend})` : ''}.<//>`
      : !bridges.data ? (bridges.error ? null : html`<${Loading} />`)
      : bridges.data.length === 0 ? html`<${Empty}>No bridges. A bridge lets several interfaces share one network, such as the wired ports of a router.<//>`
      : html`<${Table}>
        <thead><tr><th>Bridge</th><th>Ports</th><th>Spanning tree</th><th>Addresses</th></tr></thead>
        <tbody>${bridges.data.map(b => html`<tr key=${b.name}>
          <td><a class="cell-main mono" href=${href(`/bridges/${encodeURIComponent(b.name)}`)}>${b.name}</a></td>
          <td class="mono small">${b.ports.length ? b.ports.join(', ') : html`<span class="muted">None</span>`}</td>
          <td>${b.stp ? html`<${Badge} variant="info">On<//>` : html`<span class="muted">Off</span>`}</td>
          <td><a href=${href(`/interfaces/${encodeURIComponent(b.name)}/settings`)}>Address settings</a></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
