import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Empty, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges, routeURL } from '@lsongdev/panel/forms.js';
import { useApi, policyText } from '../zfs.js';

// New dataset under ?parent=. Everything but the name is inherited unless set.
export default function DatasetNew() {
  const { query, href } = usePanel();
  const name = query.parent || '';
  const datasets = useApi('/datasets');
  const parent = datasets.data?.find(d => d.name === name);
  const back = name.includes('/') ? href('/dataset', { name }) : href('/pool', { name });
  return html`<${PageHeader} title="New dataset" subtitle=${name ? `Under ${name}` : null} back=${{ href: back, label: name || 'Back' }} />
    <${Alert} error=${datasets.error} />
    ${!datasets.data ? (datasets.error ? null : html`<${Loading} />`)
      : !parent ? html`<${Empty}>No dataset named ${name}.<//>`
      : parent.type !== 'filesystem' ? html`<${Alert} variant="danger">${name} is a volume, which cannot hold datasets.<//>`
      : html`<${Form} parent=${parent} />`}`;
}

function Form({ parent }) {
  const { api, navigate } = usePanel();
  const [form, setForm] = useState({ name: '', mountpoint: '', compression: '', quota: '' });
  const { busy, error, run } = useAction();
  const set = (key, value) => setForm(f => ({ ...f, [key]: value }));
  const dirty = Object.values(form).some(Boolean);
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const leave = () => navigate(parent.name.includes('/') ? routeURL('/dataset', { name: parent.name }) : routeURL('/pool', { name: parent.name }));
  const inherited = parent.mountpoint && form.name.trim() ? `${parent.mountpoint.replace(/\/$/, '')}/${form.name.trim()}` : null;
  const schedule = policyText(parent.policy);
  const submit = e => {
    e.preventDefault();
    run(async () => {
      const created = await api.post('/datasets', {
        parent: parent.name, name: form.name.trim(), mountpoint: form.mountpoint.trim(),
        compression: form.compression, quota: form.quota.trim(),
      });
      toast(`Created ${created.name}`, 'success');
      saved();
      navigate(routeURL('/dataset', { name: created.name }), { replace: true });
    });
  };
  return html`<form class="form settings-form" onSubmit=${submit}>
    <${Alert} error=${error} />
    ${schedule ? html`<${Alert} variant="info">It will follow the schedule of ${parent.policy_source && parent.policy_source !== 'local' ? parent.policy_source : parent.name} (${schedule}) and take snapshots of its own. To leave it out, set its schedule to zero afterwards.<//>` : null}
    <div class="card">
      <div class="form-fields">
        <${Field} label="Name" required hint=${`Its full name will be ${parent.name}/${form.name.trim() || '…'}`}>
          <input class="input mono" required autofocus autocomplete="off" spellcheck="false" disabled=${busy} value=${form.name} onInput=${e => set('name', e.currentTarget.value)} />
        <//>
        <div class="form-grid">
          <${Field} label="Mount point" hint=${inherited ? `Default: ${inherited}` : 'Default: under its parent'}>
            <input class="input mono" autocomplete="off" spellcheck="false" placeholder="Inherit" disabled=${busy} value=${form.mountpoint} onInput=${e => set('mountpoint', e.currentTarget.value)} />
          <//>
          <${Field} label="Compression" hint=${`Default: ${parent.compression || 'inherited'}, as its parent`}>
            <select class="input" disabled=${busy} value=${form.compression} onChange=${e => set('compression', e.currentTarget.value)}>
              <option value="">Inherit</option>
              ${['lz4', 'zstd', 'gzip', 'off'].map(c => html`<option value=${c}>${c}</option>`)}
            </select>
          <//>
          <${Field} label="Quota" hint="Most it may use, such as 500G or 2T. Default: no limit">
            <input class="input mono" autocomplete="off" spellcheck="false" disabled=${busy} value=${form.quota} onInput=${e => set('quota', e.currentTarget.value)} />
          <//>
        </div>
      </div>
    </div>
    <div class="zfs-actions">
      <button class="button button-primary" type="submit" disabled=${busy || !form.name.trim()}>${busy ? 'Creating…' : 'Create dataset'}</button>
      <button class="button" type="button" disabled=${busy} onClick=${leave}>Cancel</button>
    </div>
  </form>`;
}
