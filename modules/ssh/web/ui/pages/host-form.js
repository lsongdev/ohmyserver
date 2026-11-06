import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Field, toast } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, returnPath, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { AUTH } from './hosts.js';

export default function HostEditor({ params }) {
  const { api, href, query } = usePanel();
  const isNew = !params.id;
  const host = useResource(signal => isNew ? Promise.resolve(null) : api.get(`/hosts/${params.id}`, { signal }), [params.id]);
  const back = returnPath(query);
  const title = isNew ? 'Add host' : host.data ? `Edit ${host.data.name}` : 'Edit host';
  return html`<${Page} title=${title} back=${{ href: href(back), label: 'Hosts' }}>
    ${!isNew && !host.data ? (host.error ? html`<${Notice} error=${host.error} />` : html`<${Loading} />`)
      : html`<${HostForm} host=${host.data || {}} back=${back} />`}
  <//>`;
}

function HostForm({ host, back }) {
  const { api, href, navigate } = usePanel();
  const isNew = !host.id;
  const [h, setH] = useState({
    name: host.name || '', address: host.address || '', port: host.port || 22, user: host.user || 'root',
    auth: host.auth || 'password', password: '', private_key: '', passphrase: '', description: host.description || '',
  });
  const [initial] = useState(() => JSON.stringify(h));
  const save = useAction();
  const test = useAction();
  const [tested, setTested] = useState(null);
  const saved = useUnsavedChanges(JSON.stringify(h) !== initial, { pending: save.busy });
  const set = patch => { setH(prev => ({ ...prev, ...patch })); setTested(null); };
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      if (isNew) await api.post('/hosts', h);
      else await api.put(`/hosts/${host.id}`, h);
      toast(isNew ? 'Host added' : 'Host saved', 'success');
      saved();
      navigate(back, { replace: true });
    });
  };
  const runTest = () => test.run(async () => {
    setTested(await api.post('/test', { id: host.id || 0, ...h }));
  });
  // Stored secrets are not sent back: leaving a field empty keeps them.
  const keep = !isNew && host.auth === h.auth;
  return html`<form class="form settings-form" onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${save.busy || test.busy}>
      <${Notice} error=${save.error || test.error} />
      <${FormSection} title="Connection">
      <div class="ssh-host-fields">
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
      <//>
      <${FormSection} title="Authentication">
      <fieldset class="field"><legend class="field-label">Method</legend><div class="choices">
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
      <//>
      <${FormSection} title="Connection test" description="Check that these settings can connect before saving.">
        <div><button class="button" type="button" onClick=${runTest}>${test.busy ? 'Connecting…' : 'Test connection'}</button></div>
      ${tested ? html`<${Notice} tone="info"><span class="test-result">Connected · ${tested.host_key_type}
        <span class="mono">${tested.host_key_sha256}</span>${tested.pinned ? '' : ' (recorded on the first connection after saving)'}</span><//>` : null}
      <//>
      </fieldset>
      <div class="button-actions">
        <button class="button button-primary" disabled=${save.busy || test.busy}>${save.busy ? 'Saving…' : isNew ? 'Add host' : 'Save changes'}</button>
        <a class="button" href=${href(back)}>Cancel</a>
      </div>
    </form>`;
}
