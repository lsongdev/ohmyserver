import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Empty, Table, Badge, Modal, toast } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { bytes } from '@lsongdev/panel/format.js';
import { useApi, policyText, ScheduleStatus } from '../zfs.js';
import { State, Capacity, ScanLine, DeviceGroups, layout, scanning } from '../components/pool.js';

const REFRESH = 15000;

// A pool: its disks and the datasets on it, with where each one's schedule and
// snapshots are.
export default function Pool() {
  const { query, href, link } = usePanel();
  const name = query.name || '';
  const list = useApi('/pools', undefined, REFRESH);
  const datasets = useApi('/datasets');
  const schedule = useApi('/schedule');
  const [dialog, setDialog] = useState(null);
  const p = list.data?.pools.find(x => x.name === name);
  const own = (datasets.data || []).filter(d => d.name === name || d.name.startsWith(name + '/'));
  const scheduled = own.some(d => policyText(d.policy));
  const scrubbing = p && scanning(p) && p.scan.function === 'SCRUB';
  const resilvering = p && scanning(p) && p.scan.function === 'RESILVER';

  return html`<${Page} title=${name || 'Pool'} subtitle=${p ? layout(p) : null}
    actions=${p?.vdevs && !resilvering ? html`<button class="button" onClick=${() => setDialog(scrubbing ? 'stop' : 'start')}>${scrubbing ? 'Cancel scrub' : 'Start scrub'}</button>` : null}>
    <${Notice} error=${list.error || datasets.error || schedule.error} />
    ${list.data?.details_error ? html`<${Notice} tone="warn">${list.data.details_error}<//>` : null}
    ${!list.data ? (list.error ? null : html`<${Loading} />`) : !p ? html`<${Empty}>No pool named ${name}.<//>` : html`
      <div class="card pool-card">
        <div class="pool-head">
          <${State} state=${p.health} />
          <span class="muted small pool-capacity">${bytes(p.alloc)} of ${bytes(p.size)} allocated (${p.capacity}%)</span>
        </div>
        <${Capacity} pool=${p} />
        ${p.capacity >= 80 ? html`<div class="pool-line muted">Above about 80% a pool slows down, and snapshots keep deleted files' space in use until they go too.</div>` : null}
        ${p.status ? html`<${Notice} tone=${p.health === 'ONLINE' ? 'info' : 'warn'}>${p.status}${p.action ? html` ${p.action}` : null}<//>` : null}
        ${p.errors ? html`<${Notice}>${p.errors} known data error${p.errors === 1 ? '' : 's'}. Run <code>zpool status -v ${p.name}</code> on the server for the affected files.<//>` : null}
        ${p.vdevs ? html`<${ScanLine} pool=${p} />` : null}
      </div>
      ${p.vdevs ? html`<section class="section">
        <h2 class="section-title">Disks</h2>
        <${DeviceGroups} pool=${p} />
      </section>` : null}
      <section class="section">
        <div class="section-row">
          <h2 class="section-title">Datasets</h2>
          <a class="button button-small" href=${href('/dataset/new', { parent: name })}>New dataset</a>
        </div>
        <${ScheduleStatus} schedule=${schedule.data} scheduled=${scheduled} />
        ${!datasets.data ? (datasets.error ? null : html`<${Loading} />`) : html`<${Table}>
          <thead><tr><th>Dataset</th><th>Used</th><th title="Space only snapshots keep in use">In snapshots</th><th>Snapshots</th><th>Schedule</th></tr></thead>
          <tbody>${own.map(d => {
            const depth = d.name.split('/').length - 1;
            return html`<tr key=${d.name}>
              <td><div style=${`padding-left:${Math.min(depth, 6) * 16}px`}>
                <a class="cell-main" href=${href('/dataset', { name: d.name })}>${d.name.split('/').pop()}</a>
                <div class="cell-sub">${d.mountpoint || d.type}</div>
              </div></td>
              <td class="nowrap">${bytes(d.used)}</td>
              <td class="nowrap">${bytes(d.snapshot_used)}</td>
              <td>${d.snapshots}</td>
              <td>
                ${d.policy_invalid ? html`<${Badge} tone="red" title=${d.policy_invalid}>Unreadable<//>` : policyText(d.policy) || html`<span class="muted">Off</span>`}
                ${d.policy_source && d.policy_source !== 'local' ? html`<div class="cell-sub">From ${d.policy_source}</div>` : null}
                ${d.schedule_error ? html`<div class="cell-sub"><${Badge} tone="red" title=${d.schedule_error}>Last run failed<//></div>` : null}
              </td>
            </tr>`;
          })}</tbody>
        <//>`}
      </section>`}
    ${dialog ? html`<${ScrubDialog} pool=${p} start=${dialog === 'start'} onClose=${() => setDialog(null)} onDone=${list.reload} />` : null}
  <//>`;
}

// ScrubDialog confirms what a scrub costs, or that canceling loses its progress.
function ScrubDialog({ pool: p, start, onClose, onDone }) {
  const { api } = usePanel();
  const { busy, error, run } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = () => run(async () => {
    await api.post('/pools/scrub', { pool: p.name, action: start ? 'start' : 'stop' });
    toast(start ? `Scrub of ${p.name} started` : `Scrub of ${p.name} canceled`, 'success');
    onDone();
    onClose();
  });
  return html`<${Modal} title=${start ? `Scrub ${p.name}?` : `Cancel the scrub of ${p.name}?`} onClose=${close} footer=${html`
    <button class="button" type="button" autofocus disabled=${busy} onClick=${close}>${start ? 'Cancel' : 'Keep scrubbing'}</button>
    <button class=${`button ${start ? 'button-primary' : 'button-danger'}`} type="button" disabled=${busy} onClick=${submit}>${busy ? 'Working…' : start ? 'Start scrub' : 'Cancel scrub'}</button>`}>
    <div class="zfs-dialog-body">
      <${Notice} error=${error} />
      ${start
        ? html`<p>This reads all ${bytes(p.alloc)} on the pool and checks it against its checksums, repairing what redundancy allows. The pool stays usable, but disks stay busy and file access may slow down until it ends, which can take hours. It can be canceled at any time.</p>`
        : html`<p>What it has checked so far is not kept: a later scrub starts over.</p>`}
    </div>
  <//>`;
}
