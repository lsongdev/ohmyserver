import { html, useState } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Badge, KV } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { routeURL } from '../lib/forms.js';
import { bytes } from '../lib/format.js';
import { useApi, policyText, ScheduleStatus } from '../zfs.js';
import { SnapshotTable, TakeSnapshotDialog } from '../components/snapshots.js';

// A dataset: its space, its schedule and its snapshots. Opened from Disks, its
// return link goes back there.
export default function Dataset() {
  const { query, href, link } = usePanel();
  const name = query.name || '';
  const pool = name.split('/')[0];
  const datasets = useApi('/datasets');
  const snapshots = useApi(name ? '/snapshots' : null, { dataset: name });
  const schedule = useApi('/schedule');
  const [taking, setTaking] = useState(false);
  const d = datasets.data?.find(x => x.name === name);
  const children = (datasets.data || []).filter(x => x.name.startsWith(name + '/') && !x.name.slice(name.length + 1).includes('/'));
  const disks = query.from === 'disks' && link?.('disks', '/');
  const files = d?.type === 'filesystem' && d.mounted && d.mountpoint && link?.('files', '/', { path: d.mountpoint });
  const changed = () => { snapshots.reload(); datasets.reload(); };
  const returnTo = routeURL('/dataset', { name, from: query.from });

  return html`<${Page} title=${name || 'Dataset'} subtitle=${d?.mountpoint || d?.type}
    back=${disks ? { href: disks, label: 'Disks' } : { href: href('/pool', { name: pool }), label: pool }}
    actions=${d ? html`<span class="zfs-actions">
      ${files ? html`<a class="button" href=${files}>Browse files</a>` : null}
      ${d.type === 'filesystem' ? html`<a class="button" href=${href('/dataset/new', { parent: name })}>New child dataset</a>` : null}
    </span>` : null}>
    <${Notice} error=${datasets.error || snapshots.error || schedule.error} />
    ${!datasets.data ? (datasets.error ? null : html`<${Loading} />`) : !d ? html`<${Empty}>No dataset named ${name}.<//>` : html`
      <div class="card">
        <${KV} items=${[
          ['Used', html`${bytes(d.used)} <span class="muted">${bytes(d.snapshot_used)} of it only in snapshots</span>`],
          ['Available', bytes(d.available)],
          ['Compression', d.compression || '—'],
          ['Quota', d.quota ? bytes(d.quota) : 'None'],
          d.mounted || d.type !== 'filesystem' ? null : ['Mounted', html`<${Badge} tone="amber">Not mounted<//>`],
        ].filter(Boolean)} />
      </div>
      <section class="section">
        <div class="section-row">
          <h2 class="section-title">Schedule</h2>
          <a class="button button-small" href=${href('/dataset/schedule', { name, from: query.from })}>Edit schedule</a>
        </div>
        <${ScheduleStatus} schedule=${schedule.data} scheduled=${!!policyText(d.policy)} />
        <div class="card">
          ${d.policy_invalid ? html`<${Badge} tone="red" title=${d.policy_invalid}>Unreadable<//>`
            : policyText(d.policy) ? html`Keeps ${policyText(d.policy)}${d.policy_source && d.policy_source !== 'local' ? html` <span class="muted">· from ${d.policy_source}</span>` : null}`
            : html`<span class="muted">No automatic snapshots</span>`}
          ${d.schedule_error ? html`<div class="pool-line"><${Badge} tone="red" title=${d.schedule_error}>Last run failed<//></div>` : null}
        </div>
      </section>
      ${children.length ? html`<section class="section">
        <h2 class="section-title">Child datasets</h2>
        <p class="zfs-children">${children.map(c => html`<a key=${c.name} class="button button-small" href=${href('/dataset', { name: c.name })}>${c.name.split('/').pop()}</a>`)}</p>
        <p class="muted small">Each has its own snapshots: a snapshot of ${name} does not include their files.</p>
      </section>` : null}
      <section class="section">
        <div class="section-row">
          <h2 class="section-title">Snapshots</h2>
          <button class="button button-small button-primary" onClick=${() => setTaking(true)}>Take snapshot</button>
        </div>
        ${!snapshots.data ? (snapshots.error ? null : html`<${Loading} />`) : snapshots.data.length === 0
          ? html`<${Empty}>No snapshots of this dataset yet.<//>`
          : html`<${SnapshotTable} snapshots=${snapshots.data} returnTo=${returnTo} showDataset=${false} onChanged=${changed} />`}
      </section>`}
    ${taking ? html`<${TakeSnapshotDialog} datasets=${datasets.data} dataset=${name} fixed=${true} onClose=${() => setTaking(false)} onCreated=${changed} />` : null}
  <//>`;
}
