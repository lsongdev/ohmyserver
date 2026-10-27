import { html, useEffect, useRef, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Modal, Field, toast, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes, datetime } from '../lib/format.js';
import { useApi, join, parent, isDir, isImage, icon, upload, ConfirmDelete } from '../files.js';

// Huge directories render their first entries; the filter finds the rest.
const SHOW = 1000;

export default function Browser() {
  const { api, client, query, setQuery, navigate } = usePanel();
  const dir = query.path || '/';
  const listing = useApi(`/list?path=${encodeURIComponent(dir)}`);
  const [selected, setSelected] = useState(new Set());
  const [filter, setFilter] = useState('');
  const [dialog, setDialog] = useState(null); // { kind, entry? }
  const [progress, setProgress] = useState(null);
  const [dragging, setDragging] = useState(false);
  const picker = useRef(null);
  const action = useAction();

  useEffect(() => { setSelected(new Set()); setFilter(''); }, [dir]);
  const open = p => navigate(`/?path=${encodeURIComponent(p)}`);
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

  const crumbs = dir.split('/').filter(Boolean);
  const subtitle = html`<nav class="crumbs" aria-label="Path">
    <a href="#" onClick=${e => { e.preventDefault(); open('/'); }}>/</a>
    ${crumbs.map((c, i) => {
      const p = '/' + crumbs.slice(0, i + 1).join('/');
      return html`${i ? html`<span class="sep">/</span>` : null}${i === crumbs.length - 1 ? html`<span>${c}</span>`
        : html`<a href="#" onClick=${e => { e.preventDefault(); open(p); }}>${c}</a>`}`;
    })}
  </nav>`;

  return html`<${Page} title="Files" subtitle=${subtitle} actions=${html`<span class="button-row">
      <button class="button" onClick=${() => setDialog({ kind: 'mkdir' })}>New folder</button>
      <button class="button" onClick=${() => setDialog({ kind: 'newfile' })}>New file</button>
      <button class="button button-primary" onClick=${() => picker.current?.click()}>Upload</button>
      <input type="file" multiple hidden ref=${picker} onChange=${e => { send([...e.currentTarget.files]); e.currentTarget.value = ''; }} />
    </span>`}>
    <${Notice} error=${action.error} />
    ${progress != null ? html`<div class="upload-progress"><div class="muted small">Uploading… ${Math.round(progress * 100)}%</div>
      <div class="meter"><span style=${`width:${progress * 100}%`}></span></div></div>` : null}
    <div class="toolbar">
      <button class="button" disabled=${dir === '/'} onClick=${() => open(parent(dir))} title="Parent folder">↑ Up</button>
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
              <span class=${`icon ${isDir(e) ? 'dir' : ''}`}>${icon(e)}</span>
              <span><button class="link-button" type="button" onClick=${() => (isDir(e) ? open(e.path) : setDialog({ kind: 'view', entry: e }))}>${e.name}</button>
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
    ${dialog?.kind === 'view' ? html`<${Viewer} entry=${dialog.entry} onClose=${() => setDialog(null)} onDownload=${download} onSaved=${listing.reload} />` : null}
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
  const submit = e => { e.preventDefault(); run.run(() => onSubmit(text.trim())); };
  return html`<${Modal} title=${title} onClose=${onClose} footer=${html`<span class="spacer"></span>
    <button class="button" type="button" onClick=${onClose}>Cancel</button>
    <button class="button button-primary" type="submit" form="prompt-form" disabled=${run.busy || !text.trim()}>${action}</button>`}>
    <form id="prompt-form" class="form" onSubmit=${submit}>
      <${Notice} error=${run.error} />
      <${Field} label=${label} hint=${hint}>
        <input class=${`input ${mono ? 'mono' : ''}`} required autofocus value=${text} onInput=${e => setText(e.currentTarget.value)} />
      <//>
    </form>
  <//>`;
}

// Viewer shows a file: images inline, text in an editor, anything else
// with its size and a download button.
function Viewer({ entry, onClose, onDownload, onSaved }) {
  const { api, client } = usePanel();
  const image = isImage(entry.name);
  const file = useApi(image ? null : `/text?path=${encodeURIComponent(entry.path)}`);
  const [text, setText] = useState(null);
  const save = useAction();
  useEffect(() => { if (file.data?.text != null) setText(file.data.text); }, [file.data]);
  const dirty = text != null && text !== file.data?.text;
  const close = () => (!dirty || window.confirm('Discard your changes?')) && onClose();
  const submit = () => save.run(async () => {
    await api.put('text', { path: entry.path, text });
    toast(`Saved ${entry.name}`, 'success');
    onSaved();
    onClose();
  });
  let body;
  if (image) body = html`<img class="preview-image" src=${client.url('download', { path: entry.path, inline: 1 }).href} alt=${entry.name} />`;
  else if (!file.data) body = file.error ? html`<${Notice} error=${file.error} />` : html`<${Loading} />`;
  else if (file.data.binary || file.data.too_large) body = html`<${Notice} tone="info">
    ${file.data.too_large ? `This file is ${bytes(file.data.size)}, too large to open here.` : 'This file is not text.'} Download it instead.<//>`;
  else body = html`<textarea class="input editor" spellcheck="false" value=${text ?? ''} onInput=${e => setText(e.currentTarget.value)} />`;
  const editable = !image && file.data && !file.data.binary && !file.data.too_large;
  return html`<${Modal} title=${entry.path} wide onClose=${close} footer=${html`
    <span class="muted small">${bytes(entry.size)} · ${entry.mode} · ${entry.owner}</span><span class="spacer"></span>
    <button class="button" type="button" onClick=${() => onDownload(entry)}>Download</button>
    ${editable ? html`<button class="button button-primary" type="button" disabled=${!dirty || save.busy} onClick=${submit}>Save</button>` : null}`}>
    <${Notice} error=${save.error} />
    ${body}
  <//>`;
}
