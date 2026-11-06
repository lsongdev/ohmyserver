import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useBusy } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Empty, Table, Badge, Field, Modal, toast, toastError } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { useApi } from '../samba.js';

export default function Users() {
  const { api } = usePanel();
  const users = useApi('/users');
  const system = useApi('/system-users');
  const [dialog, setDialog] = useState(null);
  const [busy, run] = useBusy(toastError);
  const list = users.data || [];
  const candidates = (system.data || []).filter(s => !list.some(u => u.name === s.name));

  const toggle = u => {
    if (!u.disabled && !window.confirm(`Disable ${u.name}? They can no longer connect to any share.`)) return;
    run(u.name, async () => {
      await api.post(`/users/${encodeURIComponent(u.name)}/${u.disabled ? 'enable' : 'disable'}`);
      toast(`${u.name} ${u.disabled ? 'enabled' : 'disabled'}`, 'success');
      users.reload();
    });
  };
  const remove = u => {
    if (!window.confirm(`Remove ${u.name} from Samba? Their Samba password is deleted; the system account stays.`)) return;
    run(u.name, async () => {
      await api.del(`/users/${encodeURIComponent(u.name)}`);
      toast(`${u.name} removed from Samba`, 'success');
      users.reload();
    });
  };

  return html`<${Page} title="Users" subtitle="Accounts with a Samba password; each is a system user"
    actions=${html`<button class="button button-primary" onClick=${() => setDialog({ kind: 'add' })}>Add user</button>`}>
    <${Notice} error=${users.error} />
    ${!users.data ? (users.error ? null : html`<${Loading} />`) : list.length === 0 ? html`<${Empty}>No Samba users yet. Add one to let people sign in to the shares.<//>` : html`
      <${Table}>
        <thead><tr><th>User</th><th>Status</th><th class="actions"></th></tr></thead>
        <tbody>${list.map(u => html`<tr key=${u.name}>
          <td><span class="cell-main">${u.name}</span>${u.full_name ? html`<div class="cell-sub">${u.full_name}</div>` : null}</td>
          <td><span class="tags">
            ${u.disabled ? html`<${Badge} tone="red">Disabled<//>` : html`<${Badge} tone="green">Enabled<//>`}
            ${u.locked ? html`<${Badge} tone="amber">Locked<//>` : null}
          </span></td>
          <td class="actions"><span class="row-actions">
            <button class="button button-small" disabled=${busy[u.name]} onClick=${() => setDialog({ kind: 'password', user: u })}>Set password</button>
            <button class="button button-small" disabled=${busy[u.name]} onClick=${() => toggle(u)}>${u.disabled ? 'Enable' : 'Disable'}</button>
            <button class="button button-small button-danger" disabled=${busy[u.name]} onClick=${() => remove(u)}>Remove</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
    ${dialog?.kind === 'add' ? html`<${PasswordDialog} title="Add Samba user" action="Add user" candidates=${candidates}
      onClose=${() => setDialog(null)}
      onSubmit=${async (name, password) => { await api.post('/users', { name, password }); toast(`${name} added`, 'success'); users.reload(); }} />` : null}
    ${dialog?.kind === 'password' ? html`<${PasswordDialog} title=${`Set password for ${dialog.user.name}`} action="Set password" user=${dialog.user.name}
      onClose=${() => setDialog(null)}
      onSubmit=${async (name, password) => { await api.put(`/users/${encodeURIComponent(name)}/password`, { password }); toast(`Password for ${name} changed`, 'success'); }} />` : null}
  <//>`;
}

// PasswordDialog asks for a password twice and, when adding, which system
// user gets it. A failed submit keeps it open with the error.
function PasswordDialog({ title, action, user, candidates, onSubmit, onClose }) {
  const [name, setName] = useState(user || candidates?.[0]?.name || '');
  const [password, setPassword] = useState('');
  const [again, setAgain] = useState('');
  const { busy, error, run } = useAction();
  const dirty = password !== '' || again !== '';
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const close = () => {
    if (busy || (dirty && !window.confirm('Discard the password you entered?'))) return false;
    saved();
    onClose();
  };
  const mismatch = again !== '' && again !== password;
  const submit = e => {
    e.preventDefault();
    if (mismatch) return;
    run(async () => { await onSubmit(name, password); saved(); onClose(); });
  };
  return html`<${Modal} title=${title} onClose=${close} footer=${html`    <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="password-form" disabled=${busy || !name || !password || mismatch}>${busy ? 'Saving…' : action}</button>`}>
    <form id="password-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      ${user ? null : candidates.length ? html`<${Field} label="System user" hint="Samba users sign in with a system account's name">
        <select class="input" value=${name} disabled=${busy} onChange=${e => setName(e.currentTarget.value)}>
          ${candidates.map(c => html`<option value=${c.name}>${c.name}</option>`)}
        </select>
      <//>` : html`<${Notice} tone="info">Every system user (uid 1000 and above) already has a Samba password. Create a system account first.<//>`}
      <${Field} label="Password">
        <input class="input" type="password" autocomplete="new-password" required autofocus disabled=${busy} value=${password} onInput=${e => setPassword(e.currentTarget.value)} />
      <//>
      <${Field} label="Repeat password" error=${mismatch ? 'The passwords differ' : null}>
        <input class="input" type="password" autocomplete="new-password" required disabled=${busy} value=${again} onInput=${e => setAgain(e.currentTarget.value)} />
      <//>
    </form>
  <//>`;
}
