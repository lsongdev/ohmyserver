import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useApi } from '../network.js';

// A bridge, on a page of its own: its name, whether it runs spanning tree,
// and which Ethernet interfaces are its ports. Saving puts the change on
// trial (see TrialBanner), so a port taken from the connection to the panel
// comes back by itself.
export default function Bridge({ params }) {
  const { href } = usePanel();
  const name = params.name;
  const bridge = useApi(name ? `/bridges/${encodeURIComponent(name)}` : null);
  const links = useApi('/interfaces');
  const back = { href: href('/bridges'), label: 'Bridges' };
  const ready = links.data && (!name || bridge.data);
  return html`<${Page} title=${name || 'New bridge'} subtitle=${name ? 'Bridge' : 'Join interfaces into one network'} back=${back}>
    <${Alert} error=${bridge.error || links.error} />
    ${ready ? html`<${BridgeForm} key=${bridge.data?.version || 'new'} bridge=${bridge.data} links=${links.data} />`
      : bridge.error || links.error ? null : html`<${Loading} />`}
  <//>`;
}

function BridgeForm({ bridge, links }) {
  const { api, href, navigate } = usePanel();
  const isNew = !bridge;
  const [name, setName] = useState(bridge?.name || '');
  const [stp, setStp] = useState(bridge?.stp ?? false);
  const [ports, setPorts] = useState(bridge?.ports || []);
  const [initial] = useState(() => JSON.stringify([bridge?.name || '', bridge?.stp ?? false, bridge?.ports || []]));
  const save = useAction();
  const remove = useAction();
  const changed = JSON.stringify([name, stp, ports]) !== initial;
  const release = useUnsavedChanges(changed, { pending: save.busy || remove.busy });
  const fields = save.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const other = err => (err && !Object.keys(err.fields || {}).length ? err : null);
  const edit = fn => { fn(); save.setError(null); };

  // Ethernet interfaces only. One that another bridge or other software has
  // taken is shown, but cannot be chosen, so the reason is in front of you.
  const candidates = links.filter(l => l.kind === 'ethernet');
  const unavailable = l => (l.master && l.master !== name ? `Part of ${l.master}` : !l.master && l.reason ? l.reason : '');
  const toggle = (port, on) => edit(() => setPorts(on ? [...ports, port].sort() : ports.filter(p => p !== port)));

  // A request that ends this client's connection is expected here: the
  // banner shows what the server knows, and only other failures stay.
  const send = (run, message) => async () => {
    try {
      await run();
    } catch (err) {
      if (err.code !== 'network') throw err;
    }
    release();
    toast(message, 'info');
    navigate('/bridges', { replace: true });
  };

  const submit = e => {
    e.preventDefault();
    const taken = candidates.filter(l => ports.includes(l.name) && l.master !== name && l.protected);
    if (taken.length && !window.confirm(`${taken.map(l => l.name).join(', ')} carries this machine's default route or your connection to the panel. Joining a bridge removes its own address, so the panel may become unreachable. If it does, wait two minutes: the change reverts by itself unless you keep it.\n\nApply it?`)) return;
    save.run(send(() => api.put(`/bridges/${encodeURIComponent(name.trim())}`, { version: bridge?.version || '', config: { stp, ports } }),
      'Bridge applied. Keep it to make it permanent.'));
  };

  const destroy = () => {
    const members = bridge.ports.length ? ` Its ports (${bridge.ports.join(', ')}) become ordinary interfaces that ask for an address.` : '';
    if (!window.confirm(`Remove ${bridge.name}?${members} The change reverts by itself unless you keep it within two minutes.`)) return;
    remove.run(send(() => api.del(`/bridges/${encodeURIComponent(bridge.name)}`, { query: { version: bridge.version } }),
      'Bridge removed. Keep it to make it permanent.'));
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${save.busy || remove.busy}>
      <${Alert} error=${other(save.error) || remove.error} />
      <${FormSection} title="Bridge">
        <${Field} label="Name" hint=${isNew ? 'Up to 15 characters, such as br-lan.' : 'A bridge keeps its name.'}>
          <input class="input mono" value=${name} placeholder="br-lan" disabled=${!isNew} maxlength="15" required
            onInput=${e => edit(() => setName(e.currentTarget.value))} />
        <//>
        ${error('name')}
        <${Check} label="Spanning tree (STP)" checked=${stp} onChange=${e => edit(() => setStp(e.currentTarget.checked))} />
        <p class="muted small rows-hint">Prevents loops when the ports can reach each other some other way. Leave it off when each port leads to a different place.</p>
      <//>
      <${FormSection} title="Ports" description="Ethernet interfaces that share this network. A port gives up its own address; the bridge takes it over.">
        ${candidates.length === 0 ? html`<p class="muted">This machine has no Ethernet interfaces.</p>` : html`<div class="rows" role="group" aria-label="Ports">
          ${candidates.map(l => {
            const why = unavailable(l);
            return html`<div key=${l.name}>
              <${Check} label=${html`<span class="mono">${l.name}</span>`} checked=${ports.includes(l.name)} disabled=${!!why}
                onChange=${e => toggle(l.name, e.currentTarget.checked)} />
              ${why ? html`<span class="muted small port-note"> ${why}</span>` : null}
              ${!why && l.protected ? html`<span class="muted small port-note"> Carries the default route or your connection</span>` : null}
            </div>`;
          })}
        </div>`}
        ${error('ports')}
      <//>
      ${isNew ? html`<${Alert} variant="info">A new bridge has no address. After it is kept, set one in its address settings, or add it to the DHCP server as the LAN.<//>` : null}
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || remove.busy || !changed || !name.trim()}>${save.busy ? 'Applying…' : isNew ? 'Create' : 'Apply'}</button>
      <a class="button" href=${href('/bridges')}>Cancel</a>
      ${isNew ? null : html`<a class="button" href=${href(`/interfaces/${encodeURIComponent(bridge.name)}/settings`)}>Address settings</a>
        <button type="button" class="button button-danger" disabled=${save.busy || remove.busy} onClick=${destroy}>${remove.busy ? 'Removing…' : 'Remove'}</button>`}
    </div>
  </form>`;
}
