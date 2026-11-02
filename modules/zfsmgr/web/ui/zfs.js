// ZFS snapshot manager UI helpers shared by the pages.
import { html, useState } from './lib/html.js';
import { useAction, useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Badge, Modal, Notice, toast } from './lib/ui.js';

// useApi loads a GET endpoint with an optional query; a null path loads
// nothing yet.
export function useApi(path, query) {
  const { api } = usePanel();
  const key = JSON.stringify(query || {});
  return useResource(signal => (path ? api.get(path, { signal, query }) : Promise.resolve(undefined)), [path, key]);
}

// Automatic snapshots are removed by retention; badges say so at a glance.
export function SnapshotTags({ snapshot: s }) {
  return html`<span class="tags">
    ${s.class ? html`<${Badge} tone="blue" title="Taken by a schedule; removed by its retention">Auto · ${s.class}<//>` : html`<${Badge}>Manual<//>`}
    ${s.holds > 0 ? html`<${Badge} tone="amber" title="zfs hold: cannot be deleted until released">Held<//>` : null}
    ${s.clones ? html`<${Badge} tone="amber" title=${`Cloned to ${s.clones}: cannot be deleted while the clone exists`}>Cloned<//>` : null}
  </span>`;
}

export const pinned = s => s.holds > 0 || !!s.clones;

const CLASSES = ['hourly', 'daily', 'weekly', 'monthly'];

// policyText says what a policy keeps: "24 hourly, 14 daily".
export function policyText(policy) {
  const parts = CLASSES.filter(c => policy?.[c] > 0).map(c => `${policy[c]} ${c}`);
  return parts.join(', ');
}

export { CLASSES };

// DeleteDialog confirms deleting snapshots and reports the ones that failed,
// staying open with them. Focus starts on Cancel.
export function DeleteDialog({ names, onClose, onDeleted }) {
  const { api } = usePanel();
  const [remaining, setRemaining] = useState(names);
  const [failed, setFailed] = useState([]);
  const { busy, error, run } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = () => run(async () => {
    const result = await api.post('/snapshots/destroy', { names: remaining });
    if (result.destroyed.length) {
      toast(result.destroyed.length === 1 ? `Deleted ${result.destroyed[0]}` : `Deleted ${result.destroyed.length} snapshots`, 'success');
      onDeleted?.(result.destroyed);
    }
    if (result.failed.length === 0) { onClose(); return; }
    setFailed(result.failed);
    setRemaining(result.failed.map(f => f.name));
  });
  const one = remaining.length === 1;
  return html`<${Modal} title=${one ? 'Delete this snapshot?' : `Delete ${remaining.length} snapshots?`} onClose=${close} footer=${html`
    <button class="button" type="button" autofocus disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-danger" type="button" disabled=${busy} onClick=${submit}>${busy ? 'Deleting…' : failed.length ? 'Try again' : 'Delete'}</button>`}>
    <div class="zfs-dialog-body">
      <${Notice} error=${error} />
      ${failed.length ? html`<${Notice}>${failed.length === 1 ? 'This one' : 'These'} could not be deleted:${'\n'}${failed.map(f => `${f.name}: ${f.error}\n`)}<//>` : null}
      <p>Files that exist only in ${one ? 'this snapshot' : 'these snapshots'} are lost for good. The live data is not affected.</p>
      <ul class="zfs-names">${remaining.map(n => html`<li key=${n}>${n}</li>`)}</ul>
    </div>
  <//>`;
}
