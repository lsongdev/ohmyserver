// ZFS snapshot manager UI helpers shared by the pages.
import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Badge, Alert, toast } from '@lsongdev/scripts/react/components/index.js';
import { Modal } from '@lsongdev/panel/ui.js';
import { datetime } from '@lsongdev/panel/format.js';

// useApi loads a GET endpoint with an optional query, polling every interval
// milliseconds when that is positive; a null path loads nothing yet.
export function useApi(path, query, interval) {
  const { api } = usePanel();
  const key = JSON.stringify(query || {});
  return useResource(signal => (path ? api.get(path, { signal, query }) : Promise.resolve(undefined)), [path, key], { interval });
}

// Automatic snapshots are removed by retention; badges say so at a glance.
export function SnapshotTags({ snapshot: s }) {
  return html`<span class="tags">
    ${s.class ? html`<${Badge} variant="info" title="Taken by a schedule; removed by its retention">Auto · ${s.class}<//>` : html`<${Badge}>Manual<//>`}
    ${s.holds > 0 ? html`<${Badge} variant="warning" title="zfs hold: cannot be deleted until released">Held<//>` : null}
    ${s.clones ? html`<${Badge} variant="warning" title=${`Cloned to ${s.clones}: cannot be deleted while the clone exists`}>Cloned<//>` : null}
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
      <${Alert} error=${error} />
      ${failed.length ? html`<${Alert} variant="danger">${failed.length === 1 ? 'This one' : 'These'} could not be deleted:${'\n'}${failed.map(f => `${f.name}: ${f.error}\n`)}<//>` : null}
      <p>Files that exist only in ${one ? 'this snapshot' : 'these snapshots'} are lost for good. The live data is not affected.</p>
      <ul class="zfs-names">${remaining.map(n => html`<li key=${n}>${n}</li>`)}</ul>
    </div>
  <//>`;
}

// ScheduleStatus tells how the job that runs the schedules stands, and warns
// when schedules are set but nothing would apply them. Without a job the
// module runs them itself and there is nothing to say.
export function ScheduleStatus({ schedule: s, scheduled }) {
  const { link } = usePanel();
  if (!s?.external) return null;
  const job = s.job_id ? link('cron', `/jobs/${s.job_id}`) : null;
  const name = job ? html`<a href=${job}>ZFS snapshots</a>` : 'ZFS snapshots';
  if (!s.job_id) {
    return scheduled ? html`<${Alert} variant="warning">The job ZFS snapshots was deleted, so no snapshots are taken or removed. Save a schedule or restart the panel to bring it back.<//>` : null;
  }
  if (!s.enabled) {
    return scheduled ? html`<${Alert} variant="warning">The job ${name} is turned off, so no snapshots are taken or removed.<//>` : null;
  }
  if (s.run_status && !['succeeded', 'running'].includes(s.run_status)) {
    return html`<${Alert} variant="danger">The last run of the job ${name} ended as ${s.run_status}. Its output says why.<//>`;
  }
  return html`<p class="muted small zfs-schedule">Applied by the job ${name}${s.next_run ? html`, next at ${datetime(s.next_run)}` : null}${s.last_run ? html`; last ran ${datetime(s.last_run)}` : null}.</p>`;
}
