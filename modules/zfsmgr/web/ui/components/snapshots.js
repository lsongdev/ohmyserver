// The snapshot table and the dialog that takes one, shared by the Snapshots
// page and a dataset's page.
import { html, useMemo, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Table, Field, Check, toast } from '@lsongdev/scripts/react/components/index.js';
import { Modal } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { bytes, datetime, ago } from '@lsongdev/panel/format.js';
import { SnapshotTags, DeleteDialog, pinned } from '../zfs.js';

// SnapshotTable lists snapshots with selection and deleting. returnTo is where
// a snapshot's page goes back to.
export function SnapshotTable({ snapshots, returnTo, showDataset = true, onChanged }) {
  const { href } = usePanel();
  const [selected, setSelected] = useState(() => new Set());
  const [deleting, setDeleting] = useState(null);

  // A selection only counts what is still on screen and can be deleted.
  const deletable = useMemo(() => snapshots.filter(s => !pinned(s)), [snapshots]);
  const chosen = deletable.filter(s => selected.has(s.name)).map(s => s.name);
  const toggle = name => setSelected(old => {
    const next = new Set(old);
    if (!next.delete(name)) next.add(name);
    return next;
  });
  const allChosen = deletable.length > 0 && chosen.length === deletable.length;
  const toggleAll = () => setSelected(allChosen ? new Set() : new Set(deletable.map(s => s.name)));

  return html`
    ${chosen.length ? html`<div class="zfs-selection">
      <span>${chosen.length} selected</span>
      <button class="button button-small button-danger" onClick=${() => setDeleting(chosen)}>Delete selected</button>
      <button class="button button-small" onClick=${() => setSelected(new Set())}>Clear</button>
    </div>` : null}
    <${Table}>
      <thead><tr>
        <th class="zfs-check"><input type="checkbox" aria-label="Select all shown" checked=${allChosen} disabled=${!deletable.length} onChange=${toggleAll} /></th>
        <th>Snapshot</th><th>Created</th>
        <th title="Freed if this snapshot alone is deleted">Unique</th>
        <th title="Data this snapshot gives access to">Referenced</th>
        <th>Kind</th><th class="actions"></th>
      </tr></thead>
      <tbody>${snapshots.map(s => html`<tr key=${s.name}>
        <td class="zfs-check"><input type="checkbox" aria-label=${`Select ${s.name}`} disabled=${pinned(s)}
          checked=${!pinned(s) && selected.has(s.name)} onChange=${() => toggle(s.name)} /></td>
        <td><a class="cell-main" href=${href('/snapshot', { name: s.name, return: returnTo })}>${s.short}</a>${showDataset ? html`<div class="cell-sub">${s.dataset}</div>` : null}</td>
        <td class="nowrap" title=${datetime(s.created)}>${ago(s.created)}</td>
        <td class="nowrap">${bytes(s.used)}</td>
        <td class="nowrap">${bytes(s.referenced)}</td>
        <td><${SnapshotTags} snapshot=${s} /></td>
        <td class="actions"><button class="button button-small button-danger" disabled=${pinned(s)}
          title=${pinned(s) ? 'Held or cloned snapshots cannot be deleted' : null}
          onClick=${() => setDeleting([s.name])}>Delete</button></td>
      </tr>`)}</tbody>
    <//>
    ${deleting ? html`<${DeleteDialog} names=${deleting} onClose=${() => setDeleting(null)}
      onDeleted=${gone => {
        setSelected(old => new Set([...old].filter(n => !gone.includes(n))));
        onChanged?.();
      }} />` : null}`;
}

const pad = n => String(n).padStart(2, '0');

function defaultName() {
  const d = new Date();
  return `manual-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

// TakeSnapshotDialog takes a snapshot of one of datasets; with fixed set, of
// exactly the dataset it names.
export function TakeSnapshotDialog({ datasets, dataset, fixed = false, onClose, onCreated }) {
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
  return html`<${Modal} title=${fixed ? `Take snapshot of ${target}` : 'Take snapshot'} onClose=${close} footer=${html`
    <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="snapshot-form" disabled=${busy || !name.trim()}>${busy ? 'Taking…' : 'Take snapshot'}</button>`}>
    <form id="snapshot-form" class="form" onSubmit=${submit}>
      <${Alert} error=${error} />
      ${fixed ? null : html`<${Field} label="Dataset">
        <select class="input" value=${target} disabled=${busy} onChange=${e => setTarget(e.currentTarget.value)}>
          ${datasets.map(d => html`<option value=${d.name}>${d.name}</option>`)}
        </select>
      <//>`}
      <${Field} label="Name" hint="Names starting with auto- in the schedule's format are removed by retention">
        <input class="input mono" required autofocus disabled=${busy} value=${name} onInput=${e => setName(e.currentTarget.value)} />
      <//>
      ${hasChildren ? html`<${Check} label="Also snapshot its child datasets" checked=${recursive} disabled=${busy} onChange=${e => setRecursive(e.currentTarget.checked)} />` : null}
    </form>
  <//>`;
}
