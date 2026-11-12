import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useApi } from '../network.js';

// A dial-up connection, on a page of its own: the password does not belong in
// a row or a dialog. Saving dials, and puts it on trial (see TrialBanner), so
// a login that fails, or is not kept, leaves things as they were.
export default function PPPoEForm({ params }) {
  const { href } = usePanel();
  const name = params.name;
  const pppoe = useApi('/pppoe');
  const existing = name ? pppoe.data?.connections.find(c => c.name === name) : null;
  const back = { href: href('/pppoe'), label: 'PPPoE' };
  const ready = pppoe.data && (!name || existing);
  return html`<${Page} title=${name || 'New connection'} subtitle=${name ? 'Dial-up connection' : 'Log in to your provider'} back=${back}>
    <${Alert} error=${pppoe.error} />
    ${pppoe.data && name && !existing ? html`<${Alert} variant="warning">There is no connection named ${name}, or it was made another way and cannot be changed here.<//>` : null}
    ${pppoe.data && !pppoe.data.ready ? html`<${Alert} variant="warning">This machine cannot dial yet: ${pppoe.data.missing.join(', ')} is not installed.<//>` : null}
    ${ready ? html`<${Form} key=${existing?.uuid || 'new'} existing=${existing} parents=${pppoe.data.parents} />` : pppoe.error || (pppoe.data && !ready) ? null : html`<${Loading} />`}
  <//>`;
}

function Form({ existing, parents }) {
  const { api, href, navigate } = usePanel();
  const isNew = !existing;
  const [f, setF] = useState(() => ({
    name: existing?.name || '', parent: existing?.parent || parents[parents.length - 1] || '', username: existing?.username || '',
    password: '', service: existing?.service || '', mtu: existing?.mtu ? String(existing.mtu) : '', metric: existing?.metric ? String(existing.metric) : '',
  }));
  const [initial] = useState(() => JSON.stringify(f));
  const save = useAction();
  const changed = JSON.stringify(f) !== initial;
  const release = useUnsavedChanges(changed, { pending: save.busy });
  const set = patch => { setF(prev => ({ ...prev, ...patch })); save.setError(null); };
  const fields = save.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const other = err => (err && !Object.keys(err.fields || {}).length ? err : null);

  const submit = e => {
    e.preventDefault();
    const name = f.name.trim();
    if (existing?.active && !window.confirm(`${name} is connected. Saving dials it again, and the connection drops for a moment. If the new settings fail, the old ones come back by themselves.\n\nApply them?`)) return;
    save.run(async () => {
      try {
        await api.put(`/pppoe/${encodeURIComponent(name)}`, {
          parent: f.parent, username: f.username, password: f.password, service: f.service.trim(),
          mtu: f.mtu.trim() ? Number(f.mtu) : 0, metric: f.metric.trim() ? Number(f.metric) : 0,
        });
      } catch (err) {
        // Dialing may change the route this client reached the panel by. The
        // banner shows what the server knows; only other failures stay here.
        if (err.code !== 'network') throw err;
      }
      release();
      toast('Dialing. Keep the connection to make it permanent.', 'info');
      navigate('/pppoe', { replace: true });
    });
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${save.busy}>
      <${Alert} error=${other(save.error)} />
      <${FormSection} title="Account" description="What your provider gave you.">
        <${Field} label="Name" hint=${isNew ? 'Whatever you like, such as the provider.' : 'A connection keeps its name.'}>
          <input class="input" value=${f.name} maxlength="32" required disabled=${!isNew} onInput=${e => set({ name: e.currentTarget.value })} />
        <//>
        ${error('name')}
        <${Field} label="User name"><input class="input" value=${f.username} autocomplete="off" required onInput=${e => set({ username: e.currentTarget.value })} /><//>
        ${error('username')}
        <${Field} label="Password" hint=${isNew ? 'Kept on this machine, readable only by root.' : 'Empty keeps the saved password.'}>
          <input class="input" type="password" autocomplete="new-password" value=${f.password} required=${isNew} onInput=${e => set({ password: e.currentTarget.value })} />
        <//>
        ${error('password')}
      <//>
      <${FormSection} title="Line" description="The interface the modem is plugged into.">
        <${Field} label="Dials through">
          <select class="input" value=${f.parent} onChange=${e => set({ parent: e.currentTarget.value })}>
            ${parents.map(p => html`<option key=${p} value=${p}>${p}</option>`)}
          </select>
        <//>
        ${error('parent')}
      <//>
      <${FormSection} title="Advanced">
        <${Field} label="Service name" hint="Only if your provider asks for one. Empty accepts any."><input class="input" value=${f.service} onInput=${e => set({ service: e.currentTarget.value })} /><//>
        ${error('service')}
        <${Field} label="MTU" hint="Empty is 1492, which leaves room for PPPoE on Ethernet.">
          <input class="input mono" inputmode="numeric" pattern="[0-9]*" placeholder="1492" value=${f.mtu} onInput=${e => set({ mtu: e.currentTarget.value })} />
        <//>
        ${error('mtu')}
        <${Field} label="Route priority" hint="When several interfaces have a default route, the lowest number is used. Empty is 50, ahead of Ethernet's usual 100.">
          <input class="input mono" inputmode="numeric" pattern="[0-9]*" placeholder="50" value=${f.metric} onInput=${e => set({ metric: e.currentTarget.value })} />
        <//>
        ${error('metric')}
      <//>
    </fieldset>
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || !changed || !f.name.trim()}>${save.busy ? 'Dialing…' : isNew ? 'Save and dial' : 'Save and redial'}</button>
      <a class="button" href=${href('/pppoe')}>Cancel</a>
    </div>
  </form>`;
}
