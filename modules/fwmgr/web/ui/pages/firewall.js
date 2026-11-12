import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useApi, useChanged } from '../firewall.js';

// The firewall: which interfaces face the internet, whether what leaves
// through them is translated, and what may start a connection from outside.
// Saving puts it on trial (see TrialBanner), so a firewall that cuts the
// panel off is replaced by the previous one by itself.
export default function Firewall() {
  const { link } = usePanel();
  const info = useApi('/info');
  const config = useApi(info.data?.available ? '/config' : null);
  const ifaces = useApi(info.data?.available ? '/interfaces' : null);
  const rules = useApi(info.data?.available ? '/rules' : null);
  const reload = () => { config.reload(); rules.reload(); info.reload(); };
  useChanged(reload);
  const install = link?.('packages', '/', { tab: 'search', s: 'nftables' });
  return html`<${Page} title="Firewall" subtitle="Protect the interfaces that face the internet, and share the connection">
    <${Alert} error=${info.error || config.error || ifaces.error} />
    ${!info.data ? (info.error ? null : html`<${Loading} />`)
      : !info.data.available ? html`<${Alert} variant="warning">
          <strong>nft is not installed.</strong> The firewall is made with nftables.
          ${install ? html` <a href=${install}>Search Packages for <code>nftables</code></a> and install it; no restart is needed.` : html` Install the package <code>nftables</code>; no restart is needed.`}
        <//>`
      : !config.data || !ifaces.data ? (config.error || ifaces.error ? null : html`<${Loading} />`)
      : html`<${FirewallForm} key=${JSON.stringify(config.data.config)} saved=${config.data.config} interfaces=${ifaces.data} forwarding=${info.data.forwarding} />
        <section class="section">
          <h2 class="section-title">Rules in effect</h2>
          ${rules.data?.rules ? html`<pre class="mono-block">${rules.data.rules}</pre>` : html`<p class="muted">The firewall is off: there is no table of rules.</p>`}
        </section>`}
  <//>`;
}

const fromConfig = c => ({
  enabled: c.enabled, masquerade: c.masquerade, allow_ping: c.allow_ping,
  wan: [...(c.wan || [])], allow: (c.allow || []).map(p => ({ proto: p.proto, port: String(p.port) })),
});

const toConfig = f => ({
  enabled: f.enabled, masquerade: f.masquerade, allow_ping: f.allow_ping, wan: f.wan,
  allow: f.allow.filter(p => p.port.trim()).map(p => ({ proto: p.proto, port: Number(p.port) })),
});

function FirewallForm({ saved, interfaces, forwarding }) {
  const { api, navigate, href } = usePanel();
  const [f, setF] = useState(() => fromConfig(saved));
  const [initial] = useState(() => JSON.stringify(fromConfig(saved)));
  const [custom, setCustom] = useState('');
  const [script, setScript] = useState(null);
  const preview = useAction();
  const save = useAction();
  const changed = JSON.stringify(f) !== initial;
  const release = useUnsavedChanges(changed, { pending: save.busy });
  // An edit makes an earlier preview stale.
  const set = patch => { setF(prev => ({ ...prev, ...patch })); setScript(null); save.setError(null); preview.setError(null); };
  const fields = save.error?.fields || preview.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const noFields = err => (err && !Object.keys(err.fields || {}).length ? err : null);
  const names = [...new Set([...interfaces.map(i => i.name), ...f.wan])];
  const byName = Object.fromEntries(interfaces.map(i => [i.name, i]));
  const toggle = (name, on) => set({ wan: on ? [...f.wan, name].sort() : f.wan.filter(n => n !== name) });
  const addCustom = () => { const n = custom.trim(); if (n && !f.wan.includes(n)) set({ wan: [...f.wan, n].sort() }); setCustom(''); };

  const showPreview = () => preview.run(async () => setScript((await api.post('/config/preview', toConfig(f))).script));
  const submit = e => {
    e.preventDefault();
    const own = f.enabled && f.wan.filter(n => byName[n]?.client);
    if (own?.length && !window.confirm(`${own.join(', ')} is the interface this page reached the server through. A firewall on it blocks new connections from outside, which may cut you off. If it does, wait two minutes: it reverts by itself unless you keep it.\n\nApply it?`)) return;
    save.run(async () => {
      try {
        await api.put('/config', toConfig(f));
      } catch (err) {
        // The change may have cut the very connection that asked for it. The
        // banner shows what the server knows; only other failures stay here.
        if (err.code !== 'network') throw err;
      }
      release();
      toast('Firewall applied. Keep it to make it permanent.', 'info');
      navigate('/', { replace: true });
    });
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${save.busy}>
      <${Alert} error=${noFields(save.error) || noFields(preview.error)} />
      <${FormSection} title="Firewall" description="Only the interfaces below are restricted. Traffic on every other interface, such as the local network, is left as it is.">
        <${Check} label="Turn the firewall on" checked=${f.enabled} onChange=${e => set({ enabled: e.currentTarget.checked })} />
      <//>
      <${FormSection} title="Faces the internet" description="Interfaces that connect to the provider. New connections from them to this machine, and to what is behind it, are refused unless allowed below.">
        <div class="rows" role="group" aria-label="Interfaces that face the internet">
          ${names.map(name => html`<div key=${name}>
            <${Check} label=${html`<span class="mono">${name}</span>`} checked=${f.wan.includes(name)} onChange=${e => toggle(name, e.currentTarget.checked)} />
            ${byName[name]?.uplink ? html`<span class="muted small port-note">Carries the default route</span>` : null}
            ${byName[name]?.client ? html`<span class="muted small port-note">Your connection to this page</span>` : null}
            ${!byName[name] ? html`<span class="muted small port-note">Not present now</span>` : null}
          </div>`)}
        </div>
        <div class="rows-row" style="grid-template-columns: minmax(0, 1fr) auto; margin-top: 8px">
          <input class="input mono" aria-label="Another interface name" placeholder="ppp*  (for a dial-up connection)" value=${custom}
            onInput=${e => setCustom(e.currentTarget.value)} onKeyDown=${e => { if (e.key === 'Enter') { e.preventDefault(); addCustom(); } }} />
          <button type="button" class="button" onClick=${addCustom}>Add</button>
        </div>
        ${error('wan')}
      <//>
      <${FormSection} title="Sharing" description="Translating addresses lets everything behind this machine use its connection.">
        <${Check} label="Share the connection (NAT)" checked=${f.masquerade} onChange=${e => set({ masquerade: e.currentTarget.checked })} />
        <p class="muted small">Turns IPv4 forwarding on${forwarding ? ' (it is on now)' : ''}. Turning the firewall off later leaves it as it is, since other software may need it.</p>
        <${Check} label="Answer ping from the internet" checked=${f.allow_ping} onChange=${e => set({ allow_ping: e.currentTarget.checked })} />
      <//>
      <${FormSection} title="Allowed from the internet" description="Services on this machine that anyone may connect to. Replies to what this machine started are always let in.">
        <div class="rows">
          ${f.allow.map((p, i) => html`<div class="rows-row" key=${i}>
            <select class="input" aria-label="Protocol" value=${p.proto} onChange=${e => set({ allow: f.allow.map((x, j) => (j === i ? { ...x, proto: e.currentTarget.value } : x)) })}>
              <option value="tcp">TCP</option><option value="udp">UDP</option>
            </select>
            <input class="input mono" inputmode="numeric" pattern="[0-9]*" aria-label="Port" placeholder="443" value=${p.port}
              onInput=${e => set({ allow: f.allow.map((x, j) => (j === i ? { ...x, port: e.currentTarget.value } : x)) })} />
            <button type="button" class="button button-ghost" aria-label=${`Remove port ${p.port}`} onClick=${() => set({ allow: f.allow.filter((_, j) => j !== i) })}>×</button>
          </div>`)}
          <div class="rows-actions">
            <button type="button" class="button button-small" onClick=${() => set({ allow: [...f.allow, { proto: 'tcp', port: '' }] })}>Add port</button>
          </div>
        </div>
        ${error('allow')}
      <//>
      ${script ? html`<${FormSection} title="What would be applied" description="Checked by nft, not yet in effect.">
        <pre class="mono-block">${script}</pre>
      <//>` : null}
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || !changed}>${save.busy ? 'Applying…' : 'Apply'}</button>
      <button type="button" class="button" disabled=${preview.busy || save.busy} onClick=${showPreview}>${preview.busy ? 'Checking…' : 'Preview'}</button>
      <a class="button" href=${href('/')}>Cancel</a>
    </div>
  </form>`;
}
