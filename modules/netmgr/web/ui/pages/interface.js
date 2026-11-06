import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Empty, Table, Badge, KeyValue, toast } from '@lsongdev/scripts/react/components/index.js';
import { bytes } from '@lsongdev/panel/format.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi, useChanged, useRates, LinkStatus, rate, speed, cidr } from '../network.js';

export default function InterfaceDetail({ params }) {
  const { api, href } = usePanel();
  const name = params.name;
  const path = `/interfaces/${encodeURIComponent(name)}`;
  const link = useApi(path, 3000);
  const routes = useApi('/routes', 15000);
  const resolvers = useApi('/resolvers');
  const toggle = useAction();
  const rates = useRates(link.data ? [link.data] : []);
  useChanged(link.reload);
  const l = link.data;
  const r = rates.get(name);
  const own = (routes.data || []).filter(x => x.device === name);
  const dns = resolvers.data?.links?.[name] || [];

  const setUp = up => {
    if (!up && !window.confirm(`Turn ${name} off? Everything using it loses its connection until it is turned on again.`)) return;
    toggle.run(async () => {
      await api.post(`${path}/${up ? 'up' : 'down'}`);
      toast(`${name} turned ${up ? 'on' : 'off'}`, 'success');
      link.reload();
    });
  };

  const actions = l && !l.reason ? html`
    ${l.up
      ? html`<button class="button" disabled=${toggle.busy || l.protected} title=${l.protected ? 'It carries this machine\'s default route or this connection' : null} onClick=${() => setUp(false)}>Turn off</button>`
      : html`<button class="button" disabled=${toggle.busy} onClick=${() => setUp(true)}>Turn on</button>`}
    <a class="button button-primary" href=${href(`${path}/settings`)}>Edit settings</a>` : null;

  return html`<${Page} title=${name} subtitle=${l ? [l.kind, speed(l.speed_mbps)].filter(Boolean).join(' · ') : null}
      actions=${actions}>
    <${Alert} error=${link.error || toggle.error} />
    ${!l ? (link.error ? null : html`<${Loading} />`) : html`
      ${l.reason ? html`<${Alert} variant="info">${l.reason}<//>` : null}
      <div class="grid-2">
        <div class="card">
          <h2>Status</h2>
          <${KeyValue} items=${[
            ['State', html`<span class="tags"><${LinkStatus} link=${l} />${l.uplink ? html`<${Badge} variant="info">Uplink<//>` : null}</span>`],
            ['MAC address', l.mac ? html`<span class="mono">${l.mac}</span>` : '—'],
            ['MTU', l.mtu],
            ...(l.master ? [['Part of', l.master]] : []),
          ]} />
        </div>
        <div class="card">
          <h2>Traffic</h2>
          <${KeyValue} items=${[
            ['Receiving', html`<span class="rate">${rate(r?.rx)}</span>`],
            ['Sending', html`<span class="rate">${rate(r?.tx)}</span>`],
            ['Received', `${bytes(l.rx_bytes)} · ${l.rx_packets.toLocaleString()} packets`],
            ['Sent', `${bytes(l.tx_bytes)} · ${l.tx_packets.toLocaleString()} packets`],
            ['Errors', `${l.rx_errors + l.tx_errors}, ${l.rx_dropped + l.tx_dropped} dropped`],
          ]} />
        </div>
      </div>
      <section class="section">
        <h2 class="section-title">Addresses (${l.addresses.length})</h2>
        ${l.addresses.length === 0 ? html`<${Empty}>No addresses.<//>` : html`<${Table}>
          <thead><tr><th>Address</th><th>Family</th><th>Scope</th><th>Assigned</th></tr></thead>
          <tbody>${l.addresses.map(a => html`<tr key=${cidr(a)}>
            <td class="mono">${cidr(a)}</td>
            <td>${a.family === 'inet6' ? 'IPv6' : 'IPv4'}</td>
            <td>${a.scope}</td>
            <td>${a.dynamic || a.scope === 'link' ? 'Automatically' : 'Statically'}</td>
          </tr>`)}</tbody>
        <//>`}
      </section>
      <section class="section">
        <h2 class="section-title">Routes (${own.length})</h2>
        ${own.length === 0 ? html`<${Empty}>No routes leave through this interface.<//>` : html`<${Table}>
          <thead><tr><th>Destination</th><th>Via</th><th>From</th></tr></thead>
          <tbody>${own.map((x, i) => html`<tr key=${i}>
            <td class="mono">${x.destination}</td>
            <td class="mono">${x.gateway || html`<span class="muted">direct</span>`}</td>
            <td>${x.protocol || '—'}</td>
          </tr>`)}</tbody>
        <//>`}
      </section>
      ${dns.length ? html`<section class="section">
        <h2 class="section-title">DNS servers</h2>
        <div class="card mono small">${dns.join(', ')}</div>
      </section>` : null}`}
  <//>`;
}
