import { html } from '@lsongdev/panel/html.js';
import { Alert, Loading, Empty, Table, Badge } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi, useChanged, useRates, LinkStatus, rate, speed, globalAddresses, cidr } from '../network.js';

export default function Interfaces() {
  const { href } = usePanel();
  const links = useApi('/interfaces', 3000);
  const info = useApi('/info');
  const rates = useRates(links.data);
  useChanged(links.reload);
  const backend = info.data?.backend;
  return html`<${Page} title="Interfaces" subtitle=${backend ? `Settings managed by ${backend}` : 'Traffic refreshes every few seconds'}>
    ${info.data && !backend ? html`<${Alert} variant="info">Neither NetworkManager nor systemd-networkd runs on this machine, so this page can show interfaces but not change them.<//>` : null}
    <${Alert} error=${links.error} />
    ${!links.data ? (links.error ? null : html`<${Loading} />`) : links.data.length === 0 ? html`<${Empty}>No interfaces.<//>` : html`<${Table}>
      <thead><tr><th>Interface</th><th>Status</th><th>Addresses</th><th>Received</th><th>Sent</th></tr></thead>
      <tbody>${links.data.map(l => {
        const addresses = globalAddresses(l);
        const r = rates.get(l.name);
        return html`<tr key=${l.name}>
          <td>
            <a class="cell-main mono" href=${href(`/interfaces/${encodeURIComponent(l.name)}`)}>${l.name}</a>
            <div class="cell-sub">${[l.kind, speed(l.speed_mbps)].filter(Boolean).join(' · ')}</div>
          </td>
          <td><span class="tags"><${LinkStatus} link=${l} />${l.uplink ? html`<${Badge} variant="info" title="This machine's default route leaves through it">Uplink<//>` : null}</span></td>
          <td class="mono small"><div class="addresses">
            ${addresses.length ? addresses.map(a => html`<span key=${cidr(a)}>${cidr(a)}</span>`) : html`<span class="muted">None</span>`}
          </div></td>
          <td class="rate">${l.up ? rate(r?.rx) : '—'}</td>
          <td class="rate">${l.up ? rate(r?.tx) : '—'}</td>
        </tr>`;
      })}</tbody>
    <//>`}
  <//>`;
}
