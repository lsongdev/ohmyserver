import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Empty, Table, Badge, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi, useChanged } from '../network.js';

// Wi-Fi: the radios, and the networks this machine knows. Scanning and
// joining are pages of their own (wifi-scan.js, wifi-connect.js), since a
// password and a list to choose from do not fit a row.
export default function Wifi() {
  const { href } = usePanel();
  const info = useApi('/info');
  const supported = info.data?.backend === 'NetworkManager';
  const wifi = useApi(supported ? '/wifi' : null, 5000);
  useChanged(wifi.reload);
  const w = wifi.data;
  return html`<${Page} title="Wi-Fi" subtitle="Join wireless networks">
    <${Alert} error=${info.error || wifi.error} />
    ${!info.data ? (info.error ? null : html`<${Loading} />`)
      : !supported ? html`<${Alert} variant="info">Wi-Fi is managed through NetworkManager, which is not what runs on this machine${info.data.backend ? ` (${info.data.backend})` : ''}.<//>`
      : !w ? (wifi.error ? null : html`<${Loading} />`)
      : w.devices.length === 0 ? html`<${Empty}>This machine has no wireless interface.<//>`
      : html`
        ${w.devices.map(d => html`<${Radio} key=${d.name} device=${d} reload=${wifi.reload} />`)}
        <${Saved} saved=${w.saved} reload=${wifi.reload} />`}
  <//>`;
}

function Radio({ device: d, reload }) {
  const { api, href } = usePanel();
  const off = useAction();
  const disconnect = () => off.run(async () => {
    await api.post(`/interfaces/${encodeURIComponent(d.name)}/down`);
    toast(`${d.name} disconnected`, 'success');
    reload();
  });
  const joined = d.state.startsWith('connected') && d.ssid;
  return html`<section class="section">
    <h2 class="section-title">${d.name}</h2>
    <div class="card">
      <div>${joined ? html`Connected to <strong>${d.ssid}</strong>` : html`<span class="muted">Not connected (${d.state})</span>`}</div>
      <div class="form-actions">
        <a class="button button-primary" href=${href(`/wifi/${encodeURIComponent(d.name)}/scan`)}>Scan for networks</a>
        <a class="button" href=${href(`/wifi/${encodeURIComponent(d.name)}/connect`)}>Hidden network</a>
        <a class="button" href=${href(`/interfaces/${encodeURIComponent(d.name)}/settings`)}>Address settings</a>
        ${joined ? html`<button class="button" disabled=${off.busy || d.protected} title=${d.protected ? 'It carries this machine\'s default route or this connection' : null} onClick=${disconnect}>Disconnect</button>` : null}
      </div>
      <${Alert} error=${off.error} />
    </div>
  </section>`;
}

const KINDS = { open: 'Open', 'wpa-psk': 'WPA2', sae: 'WPA3', unsupported: 'Other' };

function Saved({ saved, reload }) {
  const { api } = usePanel();
  const action = useAction();
  // A forgotten network cannot be brought back: its password is not kept.
  const forget = s => {
    if (!window.confirm(`Forget ${s.ssid}? Its password is deleted, and joining it again needs the password.`)) return;
    action.run(async () => {
      await api.del(`/wifi/saved/${encodeURIComponent(s.uuid)}`);
      toast(`${s.ssid} forgotten`, 'success');
      reload();
    });
  };
  return html`<section class="section">
    <h2 class="section-title">Saved networks</h2>
    <${Alert} error=${action.error} />
    ${saved.length === 0 ? html`<${Empty}>No saved networks. Scan for one to join it.<//>` : html`<${Table}>
      <thead><tr><th>Network</th><th>Security</th><th>Status</th><th></th></tr></thead>
      <tbody>${saved.map(s => html`<tr key=${s.uuid}>
        <td class="cell-main">${s.ssid}</td>
        <td>${KINDS[s.kind] || s.kind}</td>
        <td>${s.active ? html`<${Badge} variant="success">Connected<//>` : s.autoconnect ? html`<span class="muted">Joins automatically</span>` : html`<span class="muted">Manual</span>`}</td>
        <td><button class="button button-small" disabled=${action.busy} onClick=${() => forget(s)}>Forget</button></td>
      </tr>`)}</tbody>
    <//>`}
  </section>`;
}
