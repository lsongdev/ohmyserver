import { html, useEffect, useRef, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Modal, Field, toast, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { routeURL, useUnsavedChanges } from '../lib/forms.js';
import { bytes, datetime } from '../lib/format.js';
import { useApi, join, parent, isDir, icon, upload, ConfirmDelete } from '../files.js';

// Huge directories render their first entries; the filter finds the rest.
const SHOW = 1000;

export default function Browser() {
  const { api, client, query, navigate, href, setQuery } = usePanel();
  const dir = query.path || '/';
  const listing = useApi(`/list?path=${encodeURIComponent(dir)}`);
  const [path, setPath] = useState(dir);
  const [selected, setSelected] = useState(new Set());
  const filter = query.q || '';
  const setFilter = q => setQuery({ q });
  const [dialog, setDialog] = useState(null); // { kind, entry? }
  const [progress, setProgress] = useState(null);
  const [dragging, setDragging] = useState(false);
  const picker = useRef(null);
  const action = useAction();

  useEffect(() => { setSelected(new Set()); setPath(dir); }, [dir]);
  const open = p => navigate(routeURL('/', { path: p }));
  const entries = listing.data?.entries || [];
  const text = filter.trim().toLowerCase();
  const shown = text ? entries.filter(e => e.name.toLowerCase().includes(text)) : entries;
  const chosen = entries.filter(e => selected.has(e.path));
  const toggle = p => setSelected(s => { const n = new Set(s); n.has(p) ? n.delete(p) : n.add(p); return n; });
  const download = e => { window.location.href = client.url('download', { path: e.path }).href; };
  const done = message => { toast(message, 'success'); setSelected(new Set()); setDialog(null); listing.reload(); };

  const send = async (files, overwrite = false) => {
    if (!files.length) return;
    setProgress(0);
    try {
      await upload(client.url('upload', { dir, overwrite: overwrite ? 1 : '' }).href, files, setProgress);
      done(`Uploaded ${files.length === 1 ? files[0].name : `${files.length} files`}`);
    } catch (err) {
      if (err.status === 409 && !overwrite && window.confirm(`${err.message}. Replace the existing files?`)) {
        setProgress(null);
        return send(files, true);
      }
      toastError(err);
    } finally {
      setProgress(null);
    }
  };
  const remove = list => setDialog({ kind: 'delete', entries: list });
  const drop = e => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer?.files?.length) send([...e.dataTransfer.files]);
  };

  return html`<${Page} title="Files" actions=${html`<span class="button-row">
      <button class="button" onClick=${() => setDialog({ kind: 'mkdir' })}>New folder</button>
      <button class="button" onClick=${() => setDialog({ kind: 'newfile' })}>New file</button>
      <button class="button button-primary" onClick=${() => picker.current?.click()}>Upload</button>
      <input type="file" multiple hidden ref=${picker} onChange=${e => { send([...e.currentTarget.files]); e.currentTarget.value = ''; }} />
    </span>`}>
    <${Notice} error=${action.error} />
    ${progress != null ? html`<div class="upload-progress"><div class="muted small">Uploading… ${Math.round(progress * 100)}%</div>
      <div class="meter"><span style=${`width:${progress * 100}%`}></span></div></div>` : null}
    <form class="file-path" aria-label="Folder navigation" onSubmit=${e => { e.preventDefault(); open(path.replace(/\/+$/, '') || '/'); }}>
      <button class="button icon-button" type="button" onClick=${() => window.history.back()} title="Back" aria-label="Back">⬅️</button>
      <button class="button icon-button" type="button" disabled=${dir === '/'} onClick=${() => open(parent(dir))} title="Parent folder" aria-label="Parent folder">⬆️</button>
      <input class="input mono" type="text" name="path" aria-label="Folder path" required pattern="/.*" title="Enter an absolute folder path starting with /"
        value=${path} onInput=${e => setPath(e.currentTarget.value)} spellcheck="false" autocomplete="off" />
      <button class="button button-primary" type="submit">Go</button>
    </form>
    <div class="toolbar">
      <button class="button" onClick=${listing.reload} title="Refresh">Refresh</button>
      <input class="input" type="search" placeholder="Filter this folder" aria-label="Filter" value=${filter} onInput=${e => setFilter(e.currentTarget.value)} />
      ${listing.data ? html`<span class="muted small">${text ? `${shown.length} of ${entries.length}` : `${entries.length} items`}</span>` : null}
    </div>
    ${chosen.length ? html`<div class="selection">
      <strong>${chosen.length} selected</strong>
      ${chosen.length === 1 ? html`<button class="button button-small" onClick=${() => download(chosen[0])}>${isDir(chosen[0]) ? 'Download .tar.gz' : 'Download'}</button>` : null}
      <button class="button button-small" onClick=${() => setDialog({ kind: 'copy' })}>Copy to…</button>
      <button class="button button-small" onClick=${() => setDialog({ kind: 'move' })}>Move to…</button>
      <button class="button button-small button-danger" onClick=${() => remove(chosen)}>Delete</button>
      <span class="spacer"></span>
      <button class="button button-small" onClick=${() => setSelected(new Set())}>Clear</button>
    </div>` : null}
    <div class=${`drop ${dragging ? 'dragging' : ''}`} onDragOver=${e => { e.preventDefault(); setDragging(true); }}
      onDragLeave=${e => { if (!e.currentTarget.contains(e.relatedTarget)) setDragging(false); }} onDrop=${drop}>
      <${Notice} error=${listing.error} />
      ${!listing.data ? (listing.error ? null : html`<${Loading} />`) : shown.length === 0 ? html`<${Empty}>${text ? 'Nothing here matches the filter.' : 'This folder is empty. Drop files here to upload them.'}<//>` : html`
        <${Table}>
          <thead><tr>
            <th class="check"><input type="checkbox" aria-label="Select all" checked=${shown.length > 0 && shown.every(e => selected.has(e.path))}
              onChange=${e => setSelected(e.currentTarget.checked ? new Set(shown.map(x => x.path)) : new Set())} /></th>
            <th>Name</th><th class="num">Size</th><th>Modified</th><th>Permissions</th><th>Owner</th><th class="actions"></th>
          </tr></thead>
          <tbody>${shown.slice(0, SHOW).map(e => html`<tr key=${e.path}>
            <td class="check"><input type="checkbox" aria-label=${`Select ${e.name}`} checked=${selected.has(e.path)} onChange=${() => toggle(e.path)} /></td>
            <td><div class="name-cell">
              <span class="file-icon" aria-hidden="true">${icon(e)}</span>
              <span><a class="link-button" href=${isDir(e) ? href('/', { path: e.path }) : href('/view', { path: e.path, return: routeURL('/', query) })}>${e.name}</a>
                ${e.type === 'link' ? html` <span class="link-target">→ ${e.target}${e.target_is ? '' : ' (broken)'}</span>` : null}</span>
            </div></td>
            <td class="num">${isDir(e) ? '—' : bytes(e.size)}</td>
            <td class="nowrap muted">${datetime(e.modified)}</td>
            <td class="mode">${e.mode}</td>
            <td class="muted">${e.owner}${e.group && e.group !== e.owner ? `:${e.group}` : ''}</td>
            <td class="actions"><span class="button-row">
              <button class="button button-small" onClick=${() => setDialog({ kind: 'rename', entry: e })}>Rename</button>
              <button class="button button-small" onClick=${() => download(e)} title=${isDir(e) ? 'Download as .tar.gz' : 'Download'}>Download</button>
            </span></td>
          </tr>`)}</tbody>
        <//>
        ${shown.length > SHOW ? html`<p class="muted small">Showing ${SHOW} of ${shown.length}. Filter to find the others.</p>` : null}`}
    </div>
    ${dialog?.kind === 'delete' ? html`<${ConfirmDelete} entries=${dialog.entries} onClose=${() => setDialog(null)}
      onDone=${n => done(`Moved ${n} to the trash`)} />` : null}
    ${dialog?.kind === 'mkdir' ? html`<${Prompt} title="New folder" label="Name" action="Create" onClose=${() => setDialog(null)}
      onSubmit=${name => api.post('mkdir', { path: join(dir, name) }).then(() => done(`Created ${name}`))} />` : null}
    ${dialog?.kind === 'newfile' ? html`<${Prompt} title="New file" label="Name" action="Create" onClose=${() => setDialog(null)}
      onSubmit=${name => api.put('text', { path: join(dir, name), text: '', create: true }).then(() => done(`Created ${name}`))} />` : null}
    ${dialog?.kind === 'rename' ? html`<${Prompt} title=${`Rename ${dialog.entry.name}`} label="New name" action="Rename" value=${dialog.entry.name}
      onClose=${() => setDialog(null)} onSubmit=${name => api.post('rename', { path: dialog.entry.path, name }).then(() => done(`Renamed to ${name}`))} />` : null}
    ${dialog?.kind === 'copy' || dialog?.kind === 'move' ? html`<${Prompt} title=${`${dialog.kind === 'copy' ? 'Copy' : 'Move'} ${chosen.length === 1 ? chosen[0].name : `${chosen.length} items`} to`}
      label="Destination folder" action=${dialog.kind === 'copy' ? 'Copy' : 'Move'} value=${dir} mono
      hint="An existing folder; items keep their names." onClose=${() => setDialog(null)}
      onSubmit=${to => api.post(dialog.kind, { paths: chosen.map(e => e.path), to }).then(() => done(`${dialog.kind === 'copy' ? 'Copied' : 'Moved'} to ${to}`))} />` : null}
  <//>`;
}

// Prompt asks for one value; a failed submit keeps it open with the error.
function Prompt({ title, label, action, value = '', hint, mono, onSubmit, onClose }) {
  const [text, setText] = useState(value);
  const run = useAction();
  const dirty = text !== value;
  const saved = useUnsavedChanges(dirty, { pending: run.busy });
  const close = () => {
    if (run.busy || (dirty && !window.confirm('Discard your unsaved changes?'))) return false;
    saved();
    onClose();
  };
  const submit = e => { e.preventDefault(); run.run(async () => { await onSubmit(text.trim()); saved(); }); };
  return html`<${Modal} title=${title} onClose=${close} footer=${html`<span class="spacer"></span>
    <button class="button" type="button" disabled=${run.busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="prompt-form" disabled=${run.busy || !text.trim()}>${action}</button>`}>
    <form id="prompt-form" class="form" onSubmit=${submit}>
      <${Notice} error=${run.error} />
      <${Field} label=${label} hint=${hint}>
        <input class=${`input ${mono ? 'mono' : ''}`} required autofocus disabled=${run.busy} value=${text} onInput=${e => setText(e.currentTarget.value)} />
      <//>
    </form>
  <//>`;
}
