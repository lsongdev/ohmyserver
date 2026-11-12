import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useApi } from '../network.js';

// Joining a network, on a page of its own: the password does not belong in a
// row or a dialog. Saving puts it on trial (see TrialBanner): if the new
// network does not work, the one before it comes back by itself.
export default function WifiConnect({ params }) {
  const { href, query } = usePanel();
  const wifi = useApi('/wifi');
  const back = { href: href(query.ssid ? `/wifi/${encodeURIComponent(params.name)}/scan` : '/wifi'), label: query.ssid ? 'Networks' : 'Wi-Fi' };
  return html`<${Page} title=${query.ssid ? `Join ${query.ssid}` : 'Join a hidden network'} subtitle=${`With ${params.name}`} back=${back}>
    <${Alert} error=${wifi.error} />
    ${wifi.data ? html`<${ConnectForm} device=${params.name} saved=${wifi.data.saved} />` : wifi.error ? null : html`<${Loading} />`}
  <//>`;
}

function ConnectForm({ device, saved }) {
  const { api, href, navigate, query } = usePanel();
  const known = query.ssid ? saved.find(s => s.ssid === query.ssid) : null;
  const [ssid, setSsid] = useState(query.ssid || '');
  const [security, setSecurity] = useState(known?.kind || (query.security && query.security !== 'unsupported' ? query.security : 'wpa-psk'));
  const [password, setPassword] = useState('');
  const [hidden, setHidden] = useState(!query.ssid);
  const save = useAction();
  const release = useUnsavedChanges(password !== '' || (!query.ssid && ssid !== ''), { pending: save.busy });
  const fields = save.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const other = err => (err && !Object.keys(err.fields || {}).length ? err : null);
  const secured = security !== 'open';

  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      try {
        // A saved network is joined as saved: its password stays where it is.
        await api.put(`/wifi/${encodeURIComponent(device)}/connection`, { ssid, security: known ? known.kind : security, password: known ? '' : password, hidden });
      } catch (err) {
        // Joining may end the connection that asked for it; the banner knows.
        if (err.code !== 'network') throw err;
      }
      release();
      toast('Joining. Keep the connection to make it permanent.', 'info');
      navigate('/wifi', { replace: true });
    });
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${save.busy}>
      <${Alert} error=${other(save.error)} />
      <${FormSection} title="Network">
        <${Field} label="Name">
          <input class="input" value=${ssid} disabled=${!!query.ssid} maxlength="32" required onInput=${e => { setSsid(e.currentTarget.value); save.setError(null); }} />
        <//>
        ${error('ssid')}
        ${known ? html`<p class="muted small">${ssid} is saved, so it is joined with the password already kept. Forget it on the Wi-Fi page to give it a new one.</p>` : html`
          ${query.ssid ? null : html`<${Field} label="Security">
            <select class="input" value=${security} onChange=${e => { setSecurity(e.currentTarget.value); save.setError(null); }}>
              <option value="wpa-psk">WPA2 password</option>
              <option value="sae">WPA3 password</option>
              <option value="open">Open, no password</option>
            </select>
          <//>`}
          ${error('security')}
          ${secured ? html`<${Field} label="Password" hint="8 to 63 characters. It is kept on this machine, readable only by root.">
            <input class="input" type="password" autocomplete="new-password" value=${password} required onInput=${e => { setPassword(e.currentTarget.value); save.setError(null); }} />
          <//>` : null}
          ${error('password')}`}
        ${query.ssid ? null : html`<${Check} label="This network does not announce its name" checked=${hidden} onChange=${e => setHidden(e.currentTarget.checked)} />`}
      <//>
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || !ssid.trim()}>${save.busy ? 'Joining…' : 'Join'}</button>
      <a class="button" href=${href('/wifi')}>Cancel</a>
    </div>
  </form>`;
}
