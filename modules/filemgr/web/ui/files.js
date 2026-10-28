// File manager UI helpers shared by the pages.
import { html, useEffect, useState } from './lib/html.js';
import { useResource, useAction } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Modal, Notice, Loading, Field } from './lib/ui.js';
import { bytes } from './lib/format.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

export const join = (dir, name) => (dir === '/' ? '' : dir) + '/' + name;
export const parent = p => (p === '/' ? '/' : p.slice(0, p.lastIndexOf('/')) || '/');
export const isDir = e => e.type === 'dir' || e.target_is === 'dir';

const IMAGES = /\.(png|jpe?g|gif|webp|avif|bmp|ico|svg)$/i;
export const isImage = name => IMAGES.test(name);

export function icon(e) {
  if (isDir(e)) return '📁';
  if (e.type === 'link') return '🔗';
  if (isImage(e.name)) return '🖼️';
  if (/\.(zip|tar|gz|bz2|xz|rar|7z|tgz|zst)$/i.test(e.name)) return '📦';
  return '📄';
}

// upload sends files to dir with progress (0..1), resolving with the API's
// answer; a conflict rejects with status 409.
export function upload(url, files, onProgress) {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    for (const file of files) form.append('file', file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    xhr.upload.onprogress = e => e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      let body = null;
      try { body = JSON.parse(xhr.responseText); } catch { /* not JSON */ }
      if (xhr.status >= 200 && xhr.status < 300) return resolve(body?.data);
      const err = new Error(body?.error?.message || `Upload failed (HTTP ${xhr.status})`);
      err.status = xhr.status;
      reject(err);
    };
    xhr.onerror = () => reject(new Error('Upload failed: the connection dropped'));
    xhr.send(form);
  });
}

// count formats "1 file", "150 files".
export const count = (n, noun) => `${n.toLocaleString()} ${noun}${n === 1 ? '' : 's'}`;

// A deletion this large asks for the name to be typed, not just a click.
const BIG_FILES = 100, BIG_BYTES = 1e9;

// ConfirmDelete previews what deleting entries affects, then deletes
// exactly that (the server refuses if it changed meanwhile). With purge,
// the entries are trash items and go for good; otherwise to the trash.
export function ConfirmDelete({ entries, purge = false, onClose, onDone }) {
  const { api } = usePanel();
  const paths = entries.map(e => e.path || e.id);
  const [plan, setPlan] = useState(null);
  const [typed, setTyped] = useState('');
  const preview = useAction();
  const run = useAction();
  const close = () => {
    if (run.busy) return false;
    onClose();
  };
  useEffect(() => {
    preview.run(async () => setPlan(await api.post(purge ? 'trash/preview' : 'delete/preview', { paths })));
  }, []);
  const label = entries.length === 1 ? entries[0].name : `${entries.length} items`;
  const big = plan && (plan.files >= BIG_FILES || plan.bytes >= BIG_BYTES);
  const word = purge ? 'delete' : entries.length === 1 ? entries[0].name : 'delete';
  const ready = plan && (!big && !purge || typed.trim() === word);
  const submit = e => {
    e.preventDefault();
    run.run(async () => {
      await api.post(purge ? 'trash/purge' : 'delete', { paths, expect: plan });
      onDone(label);
    });
  };
  return html`<${Modal} title=${purge ? `Delete ${label} for good` : `Delete ${label}`} onClose=${close} footer=${html`<span class="spacer"></span>
    <button class="button" type="button" autofocus=${!big && !purge} disabled=${run.busy} onClick=${close}>Cancel</button>
    <button class="button button-danger" type="submit" form="delete-form" disabled=${!ready || run.busy}>${purge ? 'Delete for good' : 'Move to trash'}</button>`}>
    <form id="delete-form" class="form" onSubmit=${submit}>
      <${Notice} error=${preview.error || run.error} />
      ${!plan ? (preview.error ? null : html`<${Loading}>Counting…<//>`) : html`
        <p style="margin:0">${purge ? 'This removes' : 'This moves'} <strong>${count(plan.files, 'file')}</strong>${plan.dirs ? html` in <strong>${count(plan.dirs, 'folder')}</strong>` : null}${' '}(${bytes(plan.bytes)})${purge ? html`. <strong>It cannot be undone.</strong>` : ' to the trash, from where it can be restored.'}</p>
        <ul class="small muted" style="margin:0;padding-left:18px">${plan.items.slice(0, 8).map(i => html`<li key=${i.path} class="mono">${i.path}</li>`)}
          ${plan.items.length > 8 ? html`<li>and ${plan.items.length - 8} more</li>` : null}</ul>
        ${big || purge ? html`<${Field} label=${html`Type <strong>${word}</strong> to confirm`}>
          <input class="input" autofocus disabled=${run.busy} value=${typed} onInput=${e => setTyped(e.currentTarget.value)} autocomplete="off" />
        <//>` : null}`}
    </form>
  <//>`;
}
