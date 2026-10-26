import { html, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Badge, Field, Modal, confirmDanger, toast, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { ago } from '../lib/format.js';

export const AUTH = { password: 'Password', key: 'Private key', panel: 'Panel key' };

export const target = h => `${h.user}@${h.address}${h.port === 22 ? '' : `:${h.port}`}`;

export default function Hosts() {
  const { api, href } = usePanel();
  const list = useResource(signal => api.get('/hosts', { signal }), []);
  const action = useAction();
  const [editing, setEditing] = useState(null); // a host, or {} for a new one

  const remove = h => {
    if (!confirmDanger(`Delete the host ${h.name}?`)) return;
    action.run(async () => { await api.del(`/hosts/${h.id}`); list.reload(); });
  };

  return html`<${Page} title="Hosts" subtitle="Save SSH hosts and open terminals to them in the browser."
    actions=${html`<button class="button button-primary" onClick=${() => setEditing({})}>Add host</button>`}>
    <${Notice} error=${list.error || action.error} />
    ${list.data === undefined ? (list.error ? null : html`<${Loading} />`) : !list.data.length ? html`<${Empty}>No hosts yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Address</th><th>Authentication</th><th>Host key</th><th>Last connected</th><th class="actions"></th></tr></thead>
        <tbody>${list.data.map(h => html`<tr key=${h.id}>
          <td><div class="cell-main">${h.name}</div>${h.description ? html`<div class="cell-sub">${h.description}</div>` : null}</td>
          <td class="mono">${target(h)}</td>
          <td><${Badge}>${AUTH[h.auth] || h.auth}<//></td>
          <td>${h.host_key_sha256
            ? html`<span class="mono cell-sub" title=${`${h.host_key_type} ${h.host_key_sha256}`}>${h.host_key_sha256.slice(0, 19)}…</span>`
            : html`<span class="muted">Recorded on first connection</span>`}</td>
          <td class="nowrap muted">${h.last_connected_at ? ago(h.last_connected_at) : '—'}</td>
          <td class="actions"><span class="button-row">
            <a class="button button-small button-primary" href=${href(`/hosts/${h.id}/terminal`)}>Terminal</a>
            <button class="button button-small" onClick=${() => setEditing(h)}>Edit</button>
            <button class="button button-small button-danger" disabled=${action.busy} onClick=${() => remove(h)}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
    ${editing ? html`<${HostForm} host=${editing} onClose=${() => setEditing(null)} onSaved=${() => { setEditing(null); list.reload(); }} />` : null}
  <//>`;
}

function HostForm({ host, onClose, onSaved }) {
  const { api, href } = usePanel();
  const isNew = !host.id;
  const [h, setH] = useState({
    name: host.name || '', address: host.address || '', port: host.port || 22, user: host.user || 'root',
    auth: host.auth || 'password', password: '', private_key: '', passphrase: '', description: host.description || '',
  });
  const save = useAction();
  const test = useAction();
  const [tested, setTested] = useState(null);
  const set = patch => { setH(prev => ({ ...prev, ...patch })); setTested(null); };
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      if (isNew) await api.post('/hosts', h);
      else await api.put(`/hosts/${host.id}`, h);
      onSaved();
    });
  };
  const runTest = () => test.run(async () => {
    setTested(await api.post('/test', { id: host.id || 0, ...h }));
    toast('Connected', 'success');
  });
  // Stored secrets are not sent back: leaving a field empty keeps them.
  const keep = !isNew && host.auth === h.auth;
  return html`<${Modal} wide title=${isNew ? 'Add host' : `Edit ${host.name}`} onClose=${onClose} footer=${html`
    <button class="button" type="button" disabled=${test.busy} onClick=${runTest}>${test.busy ? 'Connecting…' : 'Test connection'}</button>
    <span class="spacer"></span>
    <button class="button" type="button" onClick=${onClose}>Cancel</button>
    <button class="button button-primary" type="submit" form="host-form" disabled=${save.busy}>${isNew ? 'Add' : 'Save'}</button>`}>
    <form id="host-form" class="form" onSubmit=${submit}>
      <${Notice} error=${save.error || test.error} />
      ${tested ? html`<${Notice} tone="info"><span class="test-result">Connected · ${tested.host_key_type}
        <span class="mono">${tested.host_key_sha256}</span>${tested.pinned ? '' : ' (recorded on the first connection after saving)'}</span><//>` : null}
      <div class="form-grid">
        <${Field} label="Name" required>
          <input class="input" required value=${h.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="nas" autofocus />
        <//>
        <${Field} label="Description">
          <input class="input" value=${h.description} onInput=${e => set({ description: e.currentTarget.value })} />
        <//>
        <${Field} label="Address" required>
          <input class="input mono" required value=${h.address} onInput=${e => set({ address: e.currentTarget.value })} placeholder="192.168.1.10" />
        <//>
        <${Field} label="Port">
          <input class="input" type="number" min="1" max="65535" value=${h.port} onInput=${e => set({ port: Number(e.currentTarget.value) })} />
        <//>
        <${Field} label="User" required>
          <input class="input mono" required value=${h.user} onInput=${e => set({ user: e.currentTarget.value })} />
        <//>
      </div>
      <fieldset class="field"><span class="field-label">Authentication</span><div class="choices">
        ${Object.entries(AUTH).map(([k, label]) => html`<label class="field-check" key=${k}>
          <input type="radio" name="auth" checked=${h.auth === k} onChange=${() => set({ auth: k })} />${label}</label>`)}
      </div></fieldset>
      ${h.auth === 'password' ? html`<${Field} label="Password" required=${!keep} hint=${keep && host.has_password ? 'Leave empty to keep the saved password' : null}>
        <input class="input" type="password" autocomplete="new-password" required=${!keep} value=${h.password} onInput=${e => set({ password: e.currentTarget.value })} />
      <//>` : null}
      ${h.auth === 'key' ? html`
        <${Field} label="Private key" required=${!keep} hint=${keep && host.has_key ? 'Leave empty to keep the saved key' : 'OpenSSH or PEM format'}>
          <textarea class="input" rows="6" spellcheck="false" required=${!keep} value=${h.private_key}
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" onInput=${e => set({ private_key: e.currentTarget.value })} />
        <//>
        <${Field} label="Key passphrase" hint="For an encrypted key">
          <input class="input" type="password" autocomplete="new-password" value=${h.passphrase} onInput=${e => set({ passphrase: e.currentTarget.value })} />
        <//>` : null}
      ${h.auth === 'panel' ? html`<${Notice} tone="info">Add the <a href=${href('/key')}>panel key</a> to <code>~/.ssh/authorized_keys</code> on the host.<//>` : null}
    </form>
  <//>`;
}
