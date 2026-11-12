import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useApi } from '../network.js';

// A hotspot, on a page of its own: the radio serves a network instead of
// joining one. Starting it is on trial (see TrialBanner), so if it does not
// work, the network the radio had joined comes back by itself.
export default function WifiHotspot({ params }) {
  const { href } = usePanel();
  const name = params.name;
  const view = useApi(`/wifi/${encodeURIComponent(name)}/hotspot`);
  const back = { href: href('/wifi'), label: 'Wi-Fi' };
  return html`<${Page} title=${`Hotspot on ${name}`} subtitle="Serve a wireless network" back=${back}>
    <${Alert} error=${view.error} />
    ${view.data && !view.data.capable ? html`<${Alert} variant="warning">${name} cannot serve a network: its driver has no access point mode.<//>` : null}
    ${view.data ? html`<${Form} key=${view.data.hotspot?.uuid || 'new'} device=${name} view=${view.data} />` : view.error ? null : html`<${Loading} />`}
  <//>`;
}

function Form({ device, view }) {
  const { api, href, navigate } = usePanel();
  const ap = view.hotspot;
  const [f, setF] = useState(() => ({
    ssid: ap?.ssid || '', password: '', band: ap?.band || 'bg', channel: ap?.channel ? String(ap.channel) : '',
    hidden: ap?.hidden || false, mode: ap?.bridge ? 'bridge' : 'address', address: ap?.address || '192.168.50.1/24', bridge: ap?.bridge || view.bridges[0] || '',
  }));
  const [initial] = useState(() => JSON.stringify(f));
  const save = useAction();
  const stop = useAction();
  const changed = JSON.stringify(f) !== initial;
  const release = useUnsavedChanges(changed, { pending: save.busy || stop.busy });
  const set = patch => { setF(prev => ({ ...prev, ...patch })); save.setError(null); };
  const fields = save.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const other = err => (err && !Object.keys(err.fields || {}).length ? err : null);
  const path = `/wifi/${encodeURIComponent(device)}/hotspot`;
  const busy = save.busy || stop.busy;

  const submit = e => {
    e.preventDefault();
    if (view.protected && !window.confirm(`${device} carries this machine's default route or your connection to this page. A hotspot replaces the network it has joined, so you may lose the connection. If you do, wait two minutes: it comes back by itself unless you keep the change.\n\nStart it?`)) return;
    save.run(async () => {
      try {
        await api.put(path, {
          ssid: f.ssid, password: f.password, band: f.band, channel: f.channel.trim() ? Number(f.channel) : 0, hidden: f.hidden,
          address: f.mode === 'address' ? f.address.trim() : '', bridge: f.mode === 'bridge' ? f.bridge : '',
        });
      } catch (err) {
        // The change may have ended the very connection that asked for it. The
        // banner shows what the server knows; only other failures stay here.
        if (err.code !== 'network') throw err;
      }
      release();
      toast('Starting the hotspot. Keep it to make it permanent.', 'info');
      navigate('/wifi', { replace: true });
    });
  };
  const end = () => {
    if (!window.confirm(`Stop the hotspot ${ap.ssid}? Devices connected to it lose their connection, and ${device} goes back to the networks it knows.`)) return;
    stop.run(async () => { await api.post(`${path}/stop`); release(); toast('Hotspot stopped', 'success'); navigate('/wifi', { replace: true }); });
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${busy || !view.capable}>
      <${Alert} error=${other(save.error) || stop.error} />
      <${FormSection} title="Network" description="What devices see when they look for a network to join.">
        <${Field} label="Name"><input class="input" value=${f.ssid} maxlength="32" required onInput=${e => set({ ssid: e.currentTarget.value })} /><//>
        ${error('ssid')}
        <${Field} label="Password" hint=${ap?.has_password ? 'Empty keeps the saved password. 8 to 63 characters.' : '8 to 63 characters. WPA2; a hotspot with no password is not offered.'}>
          <input class="input" type="password" autocomplete="new-password" value=${f.password} required=${!ap?.has_password} onInput=${e => set({ password: e.currentTarget.value })} />
        <//>
        ${error('password')}
        <${Check} label="Do not announce the name" checked=${f.hidden} onChange=${e => set({ hidden: e.currentTarget.checked })} />
      <//>
      <${FormSection} title="Radio" description="2.4 GHz reaches farther and works with every device; 5 GHz is faster. Some countries restrict 5 GHz until the region is set.">
        <${Field} label="Band">
          <select class="input" value=${f.band} onChange=${e => set({ band: e.currentTarget.value, channel: '' })}>
            <option value="bg">2.4 GHz</option><option value="a">5 GHz</option>
          </select>
        <//>
        ${error('band')}
        <${Field} label="Channel" hint="Empty lets the radio choose."><input class="input mono" inputmode="numeric" pattern="[0-9]*" placeholder=${f.band === 'a' ? '36' : '6'} value=${f.channel} onInput=${e => set({ channel: e.currentTarget.value })} /><//>
        ${error('channel')}
      <//>
      <${FormSection} title="Addresses" description="A hotspot hands out nothing by itself: the DHCP server does, on whichever network the hotspot is part of.">
        <${Field} label="The hotspot is">
          <select class="input" value=${f.mode} onChange=${e => set({ mode: e.currentTarget.value })}>
            <option value="address">A network of its own</option>
            <option value="bridge" disabled=${view.bridges.length === 0}>Part of a bridge${view.bridges.length === 0 ? ' (no bridges yet)' : ''}</option>
          </select>
        <//>
        ${f.mode === 'bridge'
          ? html`<${Field} label="Bridge" hint="Devices on the hotspot join the same network as the bridge's other ports.">
              <select class="input" value=${f.bridge} onChange=${e => set({ bridge: e.currentTarget.value })}>${view.bridges.map(b => html`<option key=${b} value=${b}>${b}</option>`)}</select>
            <//>`
          : html`<${Field} label="Address of the hotspot" hint="With a prefix length. Devices on the hotspot use this as their gateway.">
              <input class="input mono" value=${f.address} placeholder="192.168.50.1/24" onInput=${e => set({ address: e.currentTarget.value })} />
            <//>`}
        ${error('address')}${error('bridge')}
      <//>
      ${ap?.active ? html`<${Alert} variant="info">The hotspot is running. Saving restarts it, and devices connected to it lose their connection for a moment.<//>` : null}
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${busy || !view.capable || !changed || !f.ssid.trim()}>${save.busy ? 'Starting…' : ap?.active ? 'Save and restart' : 'Save and start'}</button>
      <a class="button" href=${href('/wifi')}>Cancel</a>
      ${ap ? html`<button type="button" class="button" disabled=${busy || view.protected && ap.active} onClick=${end}>${stop.busy ? 'Stopping…' : 'Stop hotspot'}</button>` : null}
    </div>
  </form>`;
}
