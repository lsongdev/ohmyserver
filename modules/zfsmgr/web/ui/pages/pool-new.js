import { html, useMemo, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Empty, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { Modal } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges, routeURL } from '@lsongdev/panel/forms.js';
import { bytes } from '@lsongdev/panel/format.js';
import { useApi } from '../zfs.js';

const LAYOUTS = [
  { id: 'mirror', title: 'Mirror', min: 2, text: 'Every disk holds a copy. Survives all but one failing; holds one disk’s worth.' },
  { id: 'raidz1', title: 'RAID-Z1', min: 3, text: 'Survives one disk failing. Holds all but one disk’s worth.' },
  { id: 'raidz2', title: 'RAID-Z2', min: 4, text: 'Survives two disks failing. Holds all but two disks’ worth.' },
  { id: 'raidz3', title: 'RAID-Z3', min: 5, text: 'Survives three disks failing. Holds all but three disks’ worth.' },
  { id: 'stripe', title: 'Stripe', min: 1, text: 'No redundancy: one disk failing loses everything. Holds all of them.' },
];

export default function PoolNew() {
  const { href } = usePanel();
  const disks = useApi('/disks');
  const free = (disks.data || []).filter(d => !d.in_use);
  const taken = (disks.data || []).filter(d => d.in_use);
  return html`<${PageHeader} title="Create pool" subtitle="Combine empty disks into a new pool" back=${{ href: href('/'), label: 'ZFS pools' }} />
    <${Alert} error=${disks.error} />
    ${!disks.data ? (disks.error ? null : html`<${Loading}>Checking disks…<//>`) : html`
      ${free.length ? html`<${Form} disks=${free} />` : html`<${Empty}>No empty disks on this server. A pool needs disks with nothing on them.<//>`}
      ${taken.length ? html`<section class="section">
        <h2 class="section-title">Not available</h2>
        <ul class="disk-unavailable">${taken.map(d => html`<li key=${d.name}><strong>${d.name}</strong>${d.model ? ` ${d.model}` : ''} · ${bytes(d.size)} · ${d.in_use}</li>`)}</ul>
        <p class="muted small">This page never clears a disk. To reuse one, wipe it on the server first.</p>
      </section>` : null}`}`;
}

function Form({ disks }) {
  const { api, navigate } = usePanel();
  const [name, setName] = useState('');
  const [mountpoint, setMountpoint] = useState('');
  const [chosen, setChosen] = useState(() => new Set());
  const [layout, setLayout] = useState('mirror');
  const [preview, setPreview] = useState(null);
  const { busy, error, run } = useAction();
  const dirty = !!name || !!mountpoint || chosen.size > 0;
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const min = LAYOUTS.find(l => l.id === layout).min;
  const spec = useMemo(() => ({ name: name.trim(), layout, disks: disks.filter(d => chosen.has(d.id)).map(d => d.id), mountpoint: mountpoint.trim() }), [name, layout, mountpoint, chosen, disks]);
  const toggle = id => setChosen(old => {
    const next = new Set(old);
    if (!next.delete(id)) next.add(id);
    return next;
  });
  const review = e => {
    e.preventDefault();
    run(async () => setPreview(await api.post('/pools/preview', spec)));
  };
  return html`<form class="form settings-form" onSubmit=${review}>
    <${Alert} error=${error} />
    <div class="card">
      <h2 class="section-title">Disks</h2>
      <div class="form-fields" style="gap:10px">
        ${disks.map(d => html`<label class="disk-choice" key=${d.id}>
          <input type="checkbox" disabled=${busy} checked=${chosen.has(d.id)} onChange=${() => toggle(d.id)} />
          <span class="disk-choice-main">
            <span class="cell-main">${d.name}</span> <span class="muted">${d.model || ''} · ${bytes(d.size)}${d.rotational ? ' · HDD' : ''}</span>
            <div class="cell-sub">${d.id}</div>
          </span>
        </label>`)}
      </div>
    </div>
    <div class="card">
      <h2 class="section-title">Layout</h2>
      <div class="layout-choices">
        ${LAYOUTS.map(l => {
          const short = spec.disks.length < l.min;
          return html`<label key=${l.id} class=${`layout-choice${layout === l.id ? ' selected' : ''}${short ? ' off' : ''}`}>
            <input type="radio" name="layout" disabled=${busy} checked=${layout === l.id} onChange=${() => setLayout(l.id)} />
            <span>
              <strong>${l.title}</strong> <span class="muted small">${l.min === 1 ? '1 disk or more' : `${l.min} disks or more`}</span>
              <div class="cell-sub">${l.text}</div>
            </span>
          </label>`;
        })}
      </div>
    </div>
    <div class="card">
      <div class="form-grid">
        <${Field} label="Pool name" required>
          <input class="input mono" required autocomplete="off" spellcheck="false" disabled=${busy} value=${name} onInput=${e => setName(e.currentTarget.value)} />
        <//>
        <${Field} label="Mount point" hint=${`Default: /${name.trim() || 'name'}`}>
          <input class="input mono" autocomplete="off" spellcheck="false" disabled=${busy} value=${mountpoint} onInput=${e => setMountpoint(e.currentTarget.value)} />
        <//>
      </div>
    </div>
    <div class="form-actions">
      <button class="button button-primary" type="submit" disabled=${busy || !spec.name || spec.disks.length < min}>${busy ? 'Checking…' : 'Review'}</button>
      <button class="button" type="button" disabled=${busy} onClick=${() => navigate('/')}>Cancel</button>
      ${spec.disks.length < min ? html`<span class="muted small">Choose at least ${min} disk${min === 1 ? '' : 's'} for this layout.</span>` : null}
    </div>
  </form>
  ${preview ? html`<${ConfirmDialog} spec=${spec} preview=${preview} onClose=${() => setPreview(null)}
    onCreated=${() => { saved(); navigate(routeURL('/pool', { name: spec.name }), { replace: true }); }} />` : null}`;
}

// ConfirmDialog shows what would be made, from zpool's own dry run. The pool's
// name must be typed; focus still starts on Cancel.
function ConfirmDialog({ spec, preview, onClose, onCreated }) {
  const { api } = usePanel();
  const [typed, setTyped] = useState('');
  const { busy, error, run } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = e => {
    e.preventDefault();
    if (typed !== spec.name) return;
    run(async () => {
      await api.post('/pools', spec);
      toast(`Created pool ${spec.name}`, 'success');
      onClose();
      onCreated();
    });
  };
  return html`<${Modal} title=${`Create pool ${spec.name}?`} onClose=${close} footer=${html`
    <button class="button" type="button" autofocus disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-danger" type="submit" form="pool-confirm" disabled=${busy || typed !== spec.name}>${busy ? 'Creating…' : 'Create pool'}</button>`}>
    <form id="pool-confirm" class="form" onSubmit=${submit}>
      <${Alert} error=${error} />
      <p style="margin:0">About <strong>${bytes(preview.usable)}</strong> usable. Anything on these disks becomes unreachable:</p>
      <ul class="zfs-names">${preview.disks.map(d => html`<li key=${d.id}>${d.name}${d.model ? ` ${d.model}` : ''} · ${bytes(d.size)}</li>`)}</ul>
      ${preview.warnings.map(w => html`<${Alert} key=${w} variant="warning">${w}<//>`)}
      ${preview.output ? html`<pre class="preview-output">${preview.output}</pre>` : null}
      <${Field} label=${`Type ${spec.name} to confirm`}>
        <input class="input mono" autocomplete="off" spellcheck="false" disabled=${busy} value=${typed} onInput=${e => setTyped(e.currentTarget.value)} />
      <//>
    </form>
  <//>`;
}
