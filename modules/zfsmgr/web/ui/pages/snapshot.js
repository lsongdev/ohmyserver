import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Field, Modal, Badge, KV, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useUnsavedChanges, returnPath, routeURL } from '../lib/forms.js';
import { bytes, datetime, ago } from '../lib/format.js';
import { useApi, SnapshotTags, DeleteDialog, pinned } from '../zfs.js';

export default function Snapshot() {
  const { query, navigate, href, link } = usePanel();
  const name = query.name || '';
  const detail = useApi(name ? '/snapshot' : null, { name });
  const s = detail.data;
  const [dialog, setDialog] = useState(null);
  const dataset = name.split('@')[0];
  const back = returnPath(query, routeURL('/dataset', { name: dataset }));
  const backHref = (() => {
    const [path, search = ''] = back.split('?');
    return href(path, Object.fromEntries(new URLSearchParams(search)));
  })();
  const files = s?.path && link('files', '/', { path: s.path });

  return html`<${Page} title=${s?.short || name.split('@')[1] || 'Snapshot'} subtitle=${s?.dataset || name.split('@')[0]}
    back=${{ href: backHref, label: back.startsWith('/dataset') ? dataset : 'Snapshots' }}
    actions=${s ? html`<span class="zfs-actions">
      ${files ? html`<a class="button" href=${files}>Browse files</a>` : null}
      <button class="button" onClick=${() => setDialog('rename')}>Rename</button>
      <button class="button button-danger" onClick=${() => setDialog('rollback')}>Roll back…</button>
      <button class="button button-danger" disabled=${pinned(s)} title=${pinned(s) ? 'Held or cloned snapshots cannot be deleted' : null} onClick=${() => setDialog('delete')}>Delete</button>
    </span>` : null}>
    <${Notice} error=${detail.error} />
    ${!s ? (detail.error ? null : html`<${Loading} />`) : html`
      <div class="card">
        <${KV} items=${[
          ['Created', html`${datetime(s.created)} <span class="muted">(${ago(s.created)})</span>`],
          ['Kind', html`<${SnapshotTags} snapshot=${s} />`],
          ['Unique space', html`${bytes(s.used)} <span class="muted">freed if only this one is deleted</span>`],
          ['Referenced', bytes(s.referenced)],
          ['Written', html`${bytes(s.written)} <span class="muted">since the previous snapshot</span>`],
          s.path ? ['Files', html`<code class="zfs-path">${s.path}</code>`] : null,
          s.clones ? ['Clones', html`<code class="zfs-path">${s.clones}</code>`] : null,
          ['Newer snapshots', s.newer.length],
        ].filter(Boolean)} />
      </div>
      <${Changes} key=${s.name} name=${s.name} />`}
    ${dialog === 'rename' ? html`<${RenameDialog} snapshot=${s} onClose=${() => setDialog(null)}
      onRenamed=${full => navigate(routeURL('/snapshot', { ...query, name: full }), { replace: true })} />` : null}
    ${dialog === 'rollback' ? html`<${RollbackDialog} snapshot=${s} onClose=${() => setDialog(null)} onStale=${detail.reload}
      onDone=${() => detail.reload()} />` : null}
    ${dialog === 'delete' ? html`<${DeleteDialog} names=${[s.name]} onClose=${() => setDialog(null)} onDeleted=${() => navigate(back)} />` : null}
  <//>`;
}

const KINDS = {
  '+': ['green', 'Added'],
  '-': ['red', 'Removed'],
  M: ['blue', 'Modified'],
  R: ['amber', 'Renamed'],
};

// Changes runs zfs diff on request, since it can take a while on a busy
// dataset. Volumes have no files to compare, and zfs says so.
function Changes({ name }) {
  const { api } = usePanel();
  const [diff, setDiff] = useState(null);
  const { busy, error, run } = useAction();
  const load = () => run(async () => setDiff(await api.get('/snapshot/diff', { query: { name } })));
  return html`<section class="section">
    <h2 class="section-title">Changes since this snapshot</h2>
    <${Notice} error=${error} />
    ${busy ? html`<${Loading} />` : !diff ? html`<button class="button" onClick=${load}>Compare with the live data</button>`
      : diff.changes.length === 0 ? html`<${Empty}>Nothing has changed since this snapshot.<//>` : html`
        <div class="zfs-selection">
          <span>${diff.truncated ? `First ${diff.changes.length} changes` : `${diff.changes.length} change${diff.changes.length === 1 ? '' : 's'}`}</span>
          <button class="button button-small" onClick=${load}>Compare again</button>
        </div>
        <${Table}>
          <thead><tr><th>Change</th><th>Path</th></tr></thead>
          <tbody>${diff.changes.map((c, i) => {
            const [tone, label] = KINDS[c.kind] || [undefined, c.kind];
            return html`<tr key=${i}>
              <td><${Badge} tone=${tone}>${label}<//></td>
              <td class="mono zfs-path">${c.path}${c.to ? html` → ${c.to}` : null}</td>
            </tr>`;
          })}</tbody>
        <//>`}
  </section>`;
}

function RenameDialog({ snapshot, onClose, onRenamed }) {
  const { api } = usePanel();
  const [name, setName] = useState(snapshot.short);
  const { busy, error, run } = useAction();
  const dirty = name !== snapshot.short;
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const close = () => {
    if (busy || (dirty && !window.confirm('Discard the new name?'))) return false;
    saved();
    onClose();
  };
  const submit = e => {
    e.preventDefault();
    run(async () => {
      await api.post('/snapshots/rename', { name: snapshot.name, new_name: name.trim() });
      toast(`Renamed to ${name.trim()}`, 'success');
      saved();
      onClose();
      onRenamed(`${snapshot.dataset}@${name.trim()}`);
    });
  };
  return html`<${Modal} title="Rename snapshot" onClose=${close} footer=${html`
    <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="rename-form" disabled=${busy || !dirty || !name.trim()}>${busy ? 'Renaming…' : 'Rename'}</button>`}>
    <form id="rename-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <${Field} label="Name" hint=${snapshot.class ? 'A renamed automatic snapshot is no longer removed by retention' : null}>
        <input class="input mono" required autofocus disabled=${busy} value=${name} onInput=${e => setName(e.currentTarget.value)} />
      <//>
    </form>
  <//>`;
}

// RollbackDialog is the one action here that loses live data, so the dataset
// must be typed; focus still starts on Cancel. The server refuses if the
// newer snapshots are no longer the ones listed.
function RollbackDialog({ snapshot: s, onClose, onStale, onDone }) {
  const { api } = usePanel();
  const [typed, setTyped] = useState('');
  const { busy, error, run, setError } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = e => {
    e.preventDefault();
    if (typed !== s.dataset) return;
    run(async () => {
      try {
        await api.post('/snapshots/rollback', { name: s.name, destroy_newer: s.newer.map(n => n.name) });
      } catch (err) {
        if (err.status === 409) onStale();
        throw err;
      }
      toast(`Rolled ${s.dataset} back to ${s.short}`, 'success');
      onDone();
      onClose();
    });
  };
  return html`<${Modal} title=${`Roll ${s.dataset} back?`} onClose=${close} footer=${html`
    <button class="button" type="button" autofocus disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-danger" type="submit" form="rollback-form" disabled=${busy || typed !== s.dataset}>${busy ? 'Rolling back…' : 'Roll back'}</button>`}>
    <form id="rollback-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <p>Everything written to <strong>${s.dataset}</strong> since ${datetime(s.created)} is lost for good. This cannot be undone. To get back a few files, browse the snapshot and copy them instead.</p>
      ${s.newer.length ? html`<div>
        <strong>${s.newer.length === 1 ? 'This newer snapshot is' : `These ${s.newer.length} newer snapshots are`} destroyed too:</strong>
        <ul class="zfs-names">${s.newer.map(n => html`<li key=${n.name}>${n.name}</li>`)}</ul>
      </div>` : null}
      <${Field} label=${`Type ${s.dataset} to confirm`}>
        <input class="input mono" autocomplete="off" spellcheck="false" disabled=${busy} value=${typed}
          onInput=${e => { setTyped(e.currentTarget.value); setError(null); }} />
      <//>
    </form>
  <//>`;
}
