import { html, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, Check } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { FormSection, returnPath, useUnsavedChanges } from '../lib/forms.js';
import { canon, bool, lookup, savedToast, useApi, ParamsEditor } from '../samba.js';

export default function ShareEditor({ params }) {
  const { api, href, query } = usePanel();
  const isNew = !params.name;
  // A new share needs the version of smb.conf it is added to.
  const share = useResource(signal => api.get(isNew ? '/shares' : `/shares/${encodeURIComponent(params.name)}`, { signal }), [params.name]);
  const users = useApi('/users');
  const back = returnPath(query, '/');
  const title = isNew ? 'Add share' : `Share ${params.name}`;
  return html`<${Page} title=${title} back=${{ href: href(back), label: 'Shares' }}>
    ${!share.data ? (share.error ? html`<${Notice} error=${share.error} />` : html`<${Loading} />`)
      : html`<${ShareForm} key=${share.data.version} share=${isNew ? { version: share.data.version, name: '', params: [] } : share.data}
          users=${users.data || []} back=${back} />`}
  <//>`;
}

// The parameters the form has fields for, by their canonical names with
// their synonyms; the rest are listed as they are.
const OWN = {
  path: ['path', 'directory'], comment: ['comment'],
  readonly: ['readonly', 'writeable', 'writable', 'writeok'], browseable: ['browseable', 'browsable'],
  guestok: ['guestok', 'public'], available: ['available'],
  validusers: ['validusers'], writelist: ['writelist'], forceuser: ['forceuser'], forcegroup: ['forcegroup'],
  createmask: ['createmask', 'createmode'], directorymask: ['directorymask', 'directorymode'],
};
const owned = new Set(Object.values(OWN).flat());

// The fields' Samba defaults, so the form writes a boolean only when it was
// in the file or differs from the default.
const DEFAULTS = { readonly: true, browseable: true, guestok: false, available: true };

function fromParams(params) {
  const text = k => lookup(params, OWN[k])?.value ?? '';
  const flag = (k, invert = []) => {
    const found = lookup(params, OWN[k].filter(n => !invert.includes(n)), invert);
    if (!found) return DEFAULTS[k];
    const v = bool(found.value, DEFAULTS[k]);
    return found.inverted ? !v : v;
  };
  return {
    path: text('path'), comment: text('comment'),
    readonly: flag('readonly', ['writeable', 'writable', 'writeok']),
    browseable: flag('browseable'), guestok: flag('guestok'), available: flag('available'),
    validusers: text('validusers'), writelist: text('writelist'),
    forceuser: text('forceuser'), forcegroup: text('forcegroup'),
    createmask: text('createmask'), directorymask: text('directorymask'),
    other: params.filter(p => !owned.has(canon(p.key))).map(p => ({ ...p })),
  };
}

function toParams(f, original) {
  const had = k => OWN[k].some(n => original.some(p => canon(p.key) === n));
  const out = [];
  const text = (key, name) => { if (f[name].trim()) out.push({ key, value: f[name].trim() }); };
  const flag = (key, name) => { if (had(name) || f[name] !== DEFAULTS[name]) out.push({ key, value: f[name] ? 'yes' : 'no' }); };
  text('path', 'path');
  text('comment', 'comment');
  flag('read only', 'readonly');
  flag('browseable', 'browseable');
  flag('guest ok', 'guestok');
  flag('available', 'available');
  text('valid users', 'validusers');
  text('write list', 'writelist');
  text('force user', 'forceuser');
  text('force group', 'forcegroup');
  text('create mask', 'createmask');
  text('directory mask', 'directorymask');
  return [...out, ...f.other.filter(p => p.key.trim()).map(p => ({ key: p.key.trim(), value: p.value.trim() }))];
}

function ShareForm({ share, users, back }) {
  const { api, href, navigate, link } = usePanel();
  const isNew = !share.name;
  const special = ['homes', 'printers'].includes(share.name.toLowerCase());
  const [name, setName] = useState(share.name);
  const [f, setF] = useState(() => fromParams(share.params));
  const [initial] = useState(() => JSON.stringify([share.name, fromParams(share.params)]));
  const save = useAction();
  const remove = useAction();
  const busy = save.busy || remove.busy;
  const saved = useUnsavedChanges(JSON.stringify([name, f]) !== initial, { pending: busy });
  const set = patch => setF(prev => ({ ...prev, ...patch }));

  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      const body = { version: share.version, name: name.trim(), params: toParams(f, share.params) };
      const result = isNew ? await api.post('/shares', body) : await api.put(`/shares/${encodeURIComponent(share.name)}`, body);
      savedToast(isNew ? `Share ${body.name} added` : `Share ${body.name} saved`, result);
      saved();
      navigate(back, { replace: true });
    });
  };
  const del = () => {
    if (!window.confirm(`Remove the share ${share.name}? Clients lose access to it; the folder and its files stay.`)) return;
    remove.run(async () => {
      const result = await api.del(`/shares/${encodeURIComponent(share.name)}`, { query: { version: share.version } });
      savedToast(`Share ${share.name} removed`, result);
      saved();
      navigate(back, { replace: true });
    });
  };
  const files = f.path && !f.path.includes('%') && link('files', '/', { path: f.path.trim() });
  const userNames = users.map(u => u.name);

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${busy}>
      <${Notice} error=${save.error || remove.error} />
      <${FormSection} title="Share">
        <div class="form-grid">
          <${Field} label="Name" required hint=${special ? 'A special section; renaming it makes it an ordinary share' : 'How clients see it: \\\\server\\name'}>
            <input class="input" required value=${name} maxlength="80" autofocus=${isNew} onInput=${e => setName(e.currentTarget.value)} placeholder="media" />
          <//>
          <${Field} label="Comment" hint="Shown next to the share when browsing">
            <input class="input" value=${f.comment} onInput=${e => set({ comment: e.currentTarget.value })} />
          <//>
        </div>
        <${Field} label="Folder" required=${!special} hint=${files ? html`<a href=${files}>Open in Files</a>` : special ? 'Empty uses each user\'s home folder' : 'An absolute path on this server'}>
          <input class="input mono" required=${!special} value=${f.path} placeholder="/data/media" onInput=${e => set({ path: e.currentTarget.value })} />
        <//>
        <div class="form-grid">
          <${Check} label="Read only" checked=${f.readonly} onChange=${e => set({ readonly: e.currentTarget.checked })} />
          <${Check} label="Listed when browsing the server" checked=${f.browseable} onChange=${e => set({ browseable: e.currentTarget.checked })} />
          <${Check} label="Guests may connect without a password" checked=${f.guestok} onChange=${e => set({ guestok: e.currentTarget.checked })} />
          <${Check} label="Share is on" checked=${f.available} onChange=${e => set({ available: e.currentTarget.checked })} />
        </div>
      <//>
      <${FormSection} title="Who may connect" description="Names separated by spaces; @name is a group. Empty lets every Samba user in.">
        <datalist id="samba-users">${userNames.map(n => html`<option value=${n} />`)}</datalist>
        <div class="form-grid">
          <${Field} label="Valid users" hint=${userNames.length ? `Samba users: ${userNames.join(', ')}` : null}>
            <input class="input mono" list="samba-users" value=${f.validusers} placeholder="lsong @family" onInput=${e => set({ validusers: e.currentTarget.value })} />
          <//>
          <${Field} label="Write list" hint="May write even when the share is read only">
            <input class="input mono" list="samba-users" value=${f.writelist} onInput=${e => set({ writelist: e.currentTarget.value })} />
          <//>
        </div>
      <//>
      <${FormSection} title="Files" description="How files created through the share are owned and with which permissions.">
        <div class="form-grid">
          <${Field} label="Force user" hint="Every client acts as this system user">
            <input class="input mono" value=${f.forceuser} onInput=${e => set({ forceuser: e.currentTarget.value })} />
          <//>
          <${Field} label="Force group">
            <input class="input mono" value=${f.forcegroup} onInput=${e => set({ forcegroup: e.currentTarget.value })} />
          <//>
          <${Field} label="Create mask" hint="For new files, e.g. 0664">
            <input class="input mono" value=${f.createmask} pattern="[0-7]{3,4}" placeholder="0744" onInput=${e => set({ createmask: e.currentTarget.value })} />
          <//>
          <${Field} label="Directory mask" hint="For new folders, e.g. 0775">
            <input class="input mono" value=${f.directorymask} pattern="[0-7]{3,4}" placeholder="0755" onInput=${e => set({ directorymask: e.currentTarget.value })} />
          <//>
        </div>
      <//>
      <${FormSection} title="Other parameters" description="Any other smb.conf parameter for this share, such as vfs objects. testparm checks them when you save.">
        <${ParamsEditor} rows=${f.other} onChange=${other => set({ other })} disabled=${busy} />
      <//>
    </fieldset>
    <div class="button-actions">
      <button class="button button-primary" disabled=${busy}>${save.busy ? 'Saving…' : isNew ? 'Add share' : 'Save'}</button>
      <a class="button" href=${href(back)}>Cancel</a>
      <span class="spacer"></span>
      ${isNew ? null : html`<button type="button" class="button button-danger" disabled=${busy} onClick=${del}>${remove.busy ? 'Removing…' : 'Remove'}</button>`}
    </div>
  </form>`;
}
