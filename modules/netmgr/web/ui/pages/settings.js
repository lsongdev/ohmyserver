import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { Page, useChanged } from '../network.js';

// Settings of an interface, on its own page: saving puts them on trial (see
// TrialBanner), so the page's own job ends once the change is applied.
export default function Settings({ params }) {
  const { api, href } = usePanel();
  const name = params.name;
  const path = `/interfaces/${encodeURIComponent(name)}`;
  const setting = useResource(signal => api.get(`${path}/config`, { signal }), [name]);
  useChanged(setting.reload);
  const s = setting.data;
  return html`<${Page} title=${`Settings of ${name}`} subtitle=${s ? s.target : null} back=${{ href: href(path), label: name }}>
    ${!s ? (setting.error ? html`<${Alert} error=${setting.error} />` : html`<${Loading} />`)
      : s.reason ? html`<${Alert} variant="info">${s.reason}<//>`
      : html`<${SettingsForm} key=${s.version} name=${name} path=${path} setting=${s} />`}
  <//>`;
}

function fromConfig(c) {
  return {
    dhcp4: c.dhcp4, dhcp6: c.dhcp6,
    addresses: c.addresses?.length ? [...c.addresses] : [],
    gateway4: c.gateway4 || '', gateway6: c.gateway6 || '',
    dns: c.dns?.length ? [...c.dns] : [],
    mtu: c.mtu ? String(c.mtu) : '',
    routes: (c.routes || []).map(r => ({ destination: r.destination, gateway: r.gateway || '' })),
  };
}

function toConfig(f) {
  return {
    dhcp4: f.dhcp4, dhcp6: f.dhcp6,
    addresses: f.addresses.map(a => a.trim()).filter(Boolean),
    gateway4: f.gateway4.trim(), gateway6: f.gateway6.trim(),
    dns: f.dns.map(a => a.trim()).filter(Boolean),
    mtu: f.mtu.trim() ? Number(f.mtu) : 0,
    routes: f.routes.filter(r => r.destination.trim() || r.gateway.trim()).map(r => ({ destination: r.destination.trim(), gateway: r.gateway.trim() })),
  };
}

function SettingsForm({ name, path, setting }) {
  const { api, href, navigate } = usePanel();
  const [f, setF] = useState(() => fromConfig(setting.config));
  const [initial] = useState(() => JSON.stringify(fromConfig(setting.config)));
  const [plan, setPlan] = useState(null);
  const preview = useAction();
  const save = useAction();
  const changed = JSON.stringify(f) !== initial;
  const release = useUnsavedChanges(changed, { pending: save.busy });
  // An edit makes earlier verdicts and previews stale.
  const set = patch => { setF(prev => ({ ...prev, ...patch })); setPlan(null); save.setError(null); preview.setError(null); };
  const fields = save.error?.fields || preview.error?.fields || {};
  const error = field => (fields[field] ? html`<div class="field-error" role="alert">${fields[field]}</div>` : null);
  const noFields = err => (err && !Object.keys(err.fields || {}).length ? err : null);

  const body = () => ({ version: setting.version, config: toConfig(f) });
  const showPreview = () => { save.setError(null); preview.run(async () => setPlan(await api.post(`${path}/config/preview`, body()))); };

  const submit = e => {
    e.preventDefault();
    const c = toConfig(f);
    const lonely = !c.dhcp4 && !c.dhcp6 && c.addresses.length === 0;
    if (setting.protected || lonely) {
      const why = lonely ? 'This leaves the interface with no address.' : `${name} carries this machine's default route or your connection to the panel.`;
      if (!window.confirm(`${why} The settings take effect now, and revert by themselves unless you keep them within two minutes.\n\nApply them?`)) return;
    }
    save.run(async () => {
      try {
        await api.put(`${path}/config`, body());
      } catch (err) {
        // The change may have cut the very connection that asked for it. The
        // banner shows what the server knows; only other failures stay here.
        if (err.code !== 'network') throw err;
      }
      release();
      toast('Settings applied. Keep them to make them permanent.', 'info');
      navigate(path, { replace: true });
    });
  };

  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${save.busy}>
      <${Alert} error=${noFields(save.error) || noFields(preview.error)} />
      <${FormSection} title="Addresses" description="Automatic addresses come from the network; static ones are always there. Both can be used together.">
        <div class="form-grid">
          <${Check} label="Get an IPv4 address automatically (DHCP)" checked=${f.dhcp4} onChange=${e => set({ dhcp4: e.currentTarget.checked })} />
          <${Check} label="Get an IPv6 address automatically" checked=${f.dhcp6} onChange=${e => set({ dhcp6: e.currentTarget.checked })} />
        </div>
        <div role="group" aria-labelledby="static-addresses">
          <div class="rows-title" id="static-addresses">Static addresses</div>
          <p class="muted small rows-hint">With a prefix length, IPv4 or IPv6</p>
          <${ValueRows} values=${f.addresses} placeholder="192.168.1.10/24" label="address" onChange=${addresses => set({ addresses })} />
        </div>
        ${error('addresses')}
      <//>
      <${FormSection} title="Gateway" description="Where traffic for other networks goes. Leave empty when DHCP provides it.">
        <div class="form-grid">
          <${Field} label="IPv4 gateway">
            <input class="input mono" value=${f.gateway4} placeholder="192.168.1.1" onInput=${e => set({ gateway4: e.currentTarget.value })} />
          <//>
          ${error('gateway4')}
          <${Field} label="IPv6 gateway">
            <input class="input mono" value=${f.gateway6} placeholder="fe80::1" onInput=${e => set({ gateway6: e.currentTarget.value })} />
          <//>
          ${error('gateway6')}
        </div>
      <//>
      <${FormSection} title="DNS servers" description="Empty uses the ones DHCP provides, if any.">
        <${ValueRows} values=${f.dns} placeholder="1.1.1.1" label="DNS server" onChange=${dns => set({ dns })} />
        ${error('dns')}
      <//>
      <${FormSection} title="Static routes" description="Networks reached through this interface. An empty gateway means directly attached.">
        <${RouteRows} routes=${f.routes} onChange=${routes => set({ routes })} />
        ${error('routes')}
      <//>
      ${setting.no_mtu ? null : html`<${FormSection} title="Advanced">
        <${Field} label="MTU" hint="Largest packet size in bytes. Empty leaves it to the driver.">
          <input class="input mono" inputmode="numeric" pattern="[0-9]*" value=${f.mtu} placeholder="1500" onInput=${e => set({ mtu: e.currentTarget.value })} />
        <//>
        ${error('mtu')}
      <//>`}
      ${setting.preserved?.length ? html`<${FormSection} title="Other settings" description="This form has no field for these. They stay as they are.">
        <ul class="preserved">${setting.preserved.map(p => html`<li key=${p}>${p}</li>`)}</ul>
      <//>` : null}
      ${plan ? html`<${FormSection} title="What changes" description=${plan.target}>
        <${Diff} before=${plan.before} after=${plan.after} />
      <//>` : null}
    </fieldset>
    ${setting.protected ? html`<${Alert} variant="warning">${name} carries this machine's default route or your connection. If the new settings cut you off, wait two minutes: they revert by themselves.<//>` : null}
    <div class="form-actions">
      <button class="button button-primary" disabled=${save.busy || !changed}>${save.busy ? 'Applying…' : 'Apply'}</button>
      <button type="button" class="button" disabled=${preview.busy || save.busy} onClick=${showPreview}>${preview.busy ? 'Working it out…' : 'Preview'}</button>
      <a class="button" href=${href(path)}>Cancel</a>
    </div>
  </form>`;
}

function ValueRows({ values, placeholder, label, onChange }) {
  return html`<div class="rows">
    ${values.map((v, i) => html`<div class="rows-row" key=${i}>
      <input class="input mono" aria-label=${label} placeholder=${placeholder} value=${v}
        onInput=${e => onChange(values.map((x, j) => (j === i ? e.currentTarget.value : x)))} />
      <button type="button" class="button button-ghost" aria-label=${`Remove ${v || label}`} onClick=${() => onChange(values.filter((_, j) => j !== i))}>×</button>
    </div>`)}
    <div class="rows-actions">
      <button type="button" class="button button-small" onClick=${() => onChange([...values, ''])}>Add ${label}</button>
    </div>
  </div>`;
}

function RouteRows({ routes, onChange }) {
  const change = (i, patch) => onChange(routes.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return html`<div class="rows">
    ${routes.map((r, i) => html`<div class="rows-row route" key=${i}>
      <input class="input mono" aria-label="Destination network" placeholder="10.1.0.0/16" value=${r.destination} onInput=${e => change(i, { destination: e.currentTarget.value })} />
      <input class="input mono" aria-label="Gateway" placeholder="192.168.1.254" value=${r.gateway} onInput=${e => change(i, { gateway: e.currentTarget.value })} />
      <button type="button" class="button button-ghost" aria-label=${`Remove route ${r.destination}`} onClick=${() => onChange(routes.filter((_, j) => j !== i))}>×</button>
    </div>`)}
    <div class="rows-actions">
      <button type="button" class="button button-small" onClick=${() => onChange([...routes, { destination: '', gateway: '' }])}>Add route</button>
    </div>
  </div>`;
}

// diffLines is a longest-common-subsequence line diff; these files are short.
function diffLines(before, after) {
  const a = before ? before.replace(/\n$/, '').split('\n') : [];
  const b = after ? after.replace(/\n$/, '').split('\n') : [];
  const lcs = Array.from({ length: a.length + 1 }, () => new Array(b.length + 1).fill(0));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const out = [];
  let i = 0, j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) { out.push(['same', a[i]]); i++; j++; }
    else if (j < b.length && (i === a.length || lcs[i][j + 1] >= lcs[i + 1][j])) { out.push(['add', b[j++]]); }
    else { out.push(['del', a[i++]]); }
  }
  return out;
}

function Diff({ before, after }) {
  const lines = diffLines(before, after);
  if (!lines.some(([kind]) => kind !== 'same')) return html`<p class="muted">Nothing changes.</p>`;
  const marks = { same: '  ', add: '+ ', del: '- ' };
  return html`<pre class="diff">${lines.map(([kind, text], i) => html`<span key=${i} class=${`diff-line diff-${kind}`}>${marks[kind]}${text}</span>`)}</pre>`;
}
