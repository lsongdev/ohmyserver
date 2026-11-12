import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Empty, Table, Badge } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page } from '../network.js';

const KINDS = { open: 'Open', 'wpa-psk': 'WPA2', sae: 'WPA3', unsupported: 'Not supported' };

// Signal as bars a person reads at a glance; the number is in the title.
const bars = signal => (signal >= 75 ? '▂▄▆█' : signal >= 50 ? '▂▄▆' : signal >= 25 ? '▂▄' : '▂');

// Networks the radio can hear, strongest first. Joining one is the connect
// page: this one only chooses.
export default function WifiScan({ params }) {
  const { api, href, query, setQuery } = usePanel();
  const name = params.name;
  const rescan = query.rescan === '1';
  const networks = useResource(signal => api.get(`/wifi/${encodeURIComponent(name)}/networks`, { signal, query: { rescan: rescan ? '1' : '' } }), [name, rescan]);
  const back = { href: href('/wifi'), label: 'Wi-Fi' };
  const connect = n => href(`/wifi/${encodeURIComponent(name)}/connect`, { ssid: n.ssid, security: n.kind });
  const actions = html`<button class="button" disabled=${networks.loading} onClick=${() => (rescan ? networks.reload() : setQuery({ rescan: '1' }))}>${networks.loading ? 'Scanning…' : 'Scan again'}</button>`;
  return html`<${Page} title=${`Networks heard by ${name}`} subtitle="Strongest first" back=${back} actions=${actions}>
    <${Alert} error=${networks.error} />
    ${!networks.data ? (networks.error ? null : html`<${Loading} />`)
      : networks.data.length === 0 ? html`<${Empty}>No networks found. A hidden network is joined by its name, from the Wi-Fi page.<//>`
      : html`<${Table}>
        <thead><tr><th>Network</th><th>Signal</th><th>Band</th><th>Security</th><th></th></tr></thead>
        <tbody>${networks.data.map(n => html`<tr key=${n.ssid}>
          <td class="cell-main">${n.ssid} ${n.in_use ? html`<${Badge} variant="success">Connected<//>` : null}${n.saved ? html` <${Badge} variant="info">Saved<//>` : null}</td>
          <td title=${`${n.signal}%`}><span class="mono">${bars(n.signal)}</span> <span class="muted small">${n.signal}%</span></td>
          <td>${n.band}${n.channel ? html` <span class="muted small">ch ${n.channel}</span>` : null}</td>
          <td>${KINDS[n.kind] || n.kind}</td>
          <td>${n.kind === 'unsupported' ? html`<span class="muted small">Enterprise and WEP networks cannot be joined here</span>`
            : n.in_use ? null : html`<a class="button button-small button-primary" href=${connect(n)}>${n.saved ? 'Connect' : 'Join'}</a>`}</td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
