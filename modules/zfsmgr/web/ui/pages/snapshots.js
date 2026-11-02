import { html, useMemo, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Field, Check, Modal, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useUnsavedChanges, routeURL } from '../lib/forms.js';
import { bytes, datetime, ago } from '../lib/format.js';
import { useApi, SnapshotTags, DeleteDialog, pinned } from '../zfs.js';

export default function Snapshots() {
  const { query, setQuery, href } = usePanel();
  const dataset = query.dataset || '';
  const search = query.q || '';
  const kind = query.kind || '';
  const datasets = useApi('/datasets');
  const snapshots = useApi('/snapshots', { dataset });
  const [selected, setSelected] = useState(() => new Set());
  const [dialog, setDialog] = useState(null);

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return (snapshots.data || []).filter(s =>
      (!needle || s.name.toLowerCase().includes(needle)) &&
      (!kind || (kind === 'manual' ? !s.class : s.class === kind)));
  }, [snapshots.data, search, kind]);

  // A selection only counts what is still on screen and can be deleted.
  const deletable = visible.filter(s => !pinned(s));
  const chosen = deletable.filter(s => selected.has(s.name)).map(s => s.name);
  const toggle = name => setSelected(old => {
    const next = new Set(old);
    if (!next.delete(name)) next.add(name);
    return next;
  });
  const allChosen = deletable.length > 0 && chosen.length === deletable.length;
  const toggleAll = () => setSelected(allChosen ? new Set() : new Set(deletable.map(s => s.name)));
  const returnTo = routeURL('/', query);

  return html`<${Page} title="Snapshots" subtitle="Read-only points in time of each dataset; they share space until the data diverges"
    actions=${html`<button class="button button-primary" disabled=${!datasets.data?.length} onClick=${() => setDialog({ kind: 'create' })}>Take snapshot</button>`}>
    <${Notice} error=${snapshots.error || datasets.error} />
    <div class="zfs-filters">
      <select class="input" aria-label="Dataset" value=${dataset} onChange=${e => setQuery({ dataset: e.currentTarget.value })}>
        <option value="">All datasets</option>
        ${(datasets.data || []).map(d => html`<option value=${d.name}>${d.name}</option>`)}
      </select>
      <select class="input" aria-label="Kind" value=${kind} onChange=${e => setQuery({ kind: e.currentTarget.value })}>
        <option value="">All kinds</option>
        <option value="manual">Manual</option>
        ${['hourly', 'daily', 'weekly', 'monthly'].map(c => html`<option value=${c}>Auto · ${c}</option>`)}
      </select>
      <input class="input" type="search" placeholder="Filter by name" aria-label="Filter by name" value=${search}
        onInput=${e => setQuery({ q: e.currentTarget.value })} />
    </div>
    ${chosen.length ? html`<div class="zfs-selection">
      <span>${chosen.length} selected</span>
      <button class="button button-small button-danger" onClick=${() => setDialog({ kind: 'delete', names: chosen })}>Delete selected</button>
      <button class="button button-small" onClick=${() => setSelected(new Set())}>Clear</button>
    </div>` : null}
    ${!snapshots.data ? (snapshots.error ? null : html`<${Loading} />`) : visible.length === 0 ? html`<${Empty}>${
      snapshots.data.length ? 'No snapshot matches the filter.' : 'No snapshots yet. Take one now, or set a schedule under Datasets & schedules.'
    }<//>` : html`
      <${Table}>
        <thead><tr>
          <th class="zfs-check"><input type="checkbox" aria-label="Select all shown" checked=${allChosen} disabled=${!deletable.length} onChange=${toggleAll} /></th>
          <th>Snapshot</th><th>Created</th>
          <th title="Freed if this snapshot alone is deleted">Unique</th>
          <th title="Data this snapshot gives access to">Referenced</th>
          <th>Kind</th><th class="actions"></th>
        </tr></thead>
        <tbody>${visible.map(s => html`<tr key=${s.name}>
          <td class="zfs-check"><input type="checkbox" aria-label=${`Select ${s.name}`} disabled=${pinned(s)}
            checked=${!pinned(s) && selected.has(s.name)} onChange=${() => toggle(s.name)} /></td>
          <td><a class="cell-main" href=${href('/snapshot', { name: s.name, return: returnTo })}>${s.short}</a><div class="cell-sub">${s.dataset}</div></td>
          <td class="nowrap" title=${datetime(s.created)}>${ago(s.created)}</td>
          <td class="nowrap">${bytes(s.used)}</td>
          <td class="nowrap">${bytes(s.referenced)}</td>
          <td><${SnapshotTags} snapshot=${s} /></td>
          <td class="actions"><button class="button button-small button-danger" disabled=${pinned(s)}
            title=${pinned(s) ? 'Held or cloned snapshots cannot be deleted' : null}
            onClick=${() => setDialog({ kind: 'delete', names: [s.name] })}>Delete</button></td>
        </tr>`)}</tbody>
      <//>`}
    ${dialog?.kind === 'create' ? html`<${CreateDialog} datasets=${datasets.data} dataset=${dataset}
      onClose=${() => setDialog(null)} onCreated=${() => { snapshots.reload(); datasets.reload(); }} />` : null}
    ${dialog?.kind === 'delete' ? html`<${DeleteDialog} names=${dialog.names} onClose=${() => setDialog(null)}
      onDeleted=${gone => {
        setSelected(old => new Set([...old].filter(n => !gone.includes(n))));
        snapshots.reload();
        datasets.reload();
      }} />` : null}
  <//>`;
}

const pad = n => String(n).padStart(2, '0');

function defaultName() {
  const d = new Date();
  return `manual-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

function CreateDialog({ datasets, dataset, onClose, onCreated }) {
  const { api } = usePanel();
  const [target, setTarget] = useState(dataset || datasets[0].name);
  const [initial] = useState(defaultName);
  const [name, setName] = useState(initial);
  const [recursive, setRecursive] = useState(false);
  const { busy, error, run } = useAction();
  const dirty = name !== initial;
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const hasChildren = datasets.some(d => d.name.startsWith(target + '/'));
  const close = () => {
    if (busy || (dirty && !window.confirm('Discard the name you entered?'))) return false;
    saved();
    onClose();
  };
  const submit = e => {
    e.preventDefault();
    run(async () => {
      await api.post('/snapshots', { dataset: target, name: name.trim(), recursive: hasChildren && recursive });
      toast(`Took ${target}@${name.trim()}`, 'success');
      saved();
      onCreated();
      onClose();
    });
  };
  return html`<${Modal} title="Take snapshot" onClose=${close} footer=${html`
    <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="snapshot-form" disabled=${busy || !name.trim()}>${busy ? 'Taking…' : 'Take snapshot'}</button>`}>
    <form id="snapshot-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <${Field} label="Dataset">
        <select class="input" value=${target} disabled=${busy} onChange=${e => setTarget(e.currentTarget.value)}>
          ${datasets.map(d => html`<option value=${d.name}>${d.name}</option>`)}
        </select>
      <//>
      <${Field} label="Name" hint="Names starting with auto- in the schedule's format are removed by retention">
        <input class="input mono" required autofocus disabled=${busy} value=${name} onInput=${e => setName(e.currentTarget.value)} />
      <//>
      ${hasChildren ? html`<${Check} label="Also snapshot its child datasets" checked=${recursive} disabled=${busy} onChange=${e => setRecursive(e.currentTarget.checked)} />` : null}
    </form>
  <//>`;
}
