import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Table, Badge, Status } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes } from '../lib/format.js';
import { useApi, policyText } from '../zfs.js';

function Pools({ pools }) {
  const full = pools.filter(p => p.capacity >= 80);
  return html`<div class="card">
    ${pools.map(p => html`<div class="zfs-pool" key=${p.name}>
      <div class="zfs-pool-line">
        <span class="cell-main">${p.name} <${Status} state=${p.health === 'ONLINE' ? 'healthy' : 'unhealthy'} label=${p.health} /></span>
        <span class="muted">${bytes(p.alloc)} of ${bytes(p.size)} allocated (${p.capacity}%)</span>
      </div>
      <div class="meter" role="progressbar" aria-label=${`${p.name} capacity`} aria-valuenow=${p.capacity} aria-valuemin="0" aria-valuemax="100"><span style=${`width:${p.capacity}%`}></span></div>
    </div>`)}
    ${full.length ? html`<p class="muted small">Above about 80% a pool slows down, and snapshots keep deleted files' space in use until they go too.</p>` : null}
  </div>`;
}

export default function Datasets() {
  const { href } = usePanel();
  const pools = useApi('/pools');
  const datasets = useApi('/datasets');
  return html`<${Page} title="Datasets & schedules" subtitle="How much space snapshots hold, and how many each schedule keeps">
    <${Notice} error=${pools.error || datasets.error} />
    ${pools.data?.length ? html`<${Pools} pools=${pools.data} />` : null}
    <div class="section">
      ${!datasets.data ? (datasets.error ? null : html`<${Loading} />`) : datasets.data.length === 0 ? html`<${Empty}>No datasets on this server.<//>` : html`
        <${Table}>
          <thead><tr><th>Dataset</th><th>Used</th><th title="Space only snapshots keep in use">In snapshots</th><th>Snapshots</th><th>Schedule</th><th class="actions"></th></tr></thead>
          <tbody>${datasets.data.map(d => html`<tr key=${d.name}>
            <td><span class="cell-main">${d.name}</span><div class="cell-sub">${d.mountpoint || d.type}</div></td>
            <td class="nowrap">${bytes(d.used)}</td>
            <td class="nowrap">${bytes(d.snapshot_used)}</td>
            <td><a href=${href('/', { dataset: d.name })}>${d.snapshots}</a></td>
            <td>
              ${d.policy_invalid ? html`<${Badge} tone="red" title=${d.policy_invalid}>Unreadable<//>`
                : policyText(d.policy) ? policyText(d.policy) : html`<span class="muted">Off</span>`}
              ${d.policy_source && d.policy_source !== 'local' ? html`<div class="cell-sub">From ${d.policy_source}</div>` : null}
              ${d.schedule_error ? html`<div class="cell-sub"><${Badge} tone="red" title=${d.schedule_error}>Last run failed<//></div>` : null}
            </td>
            <td class="actions"><a class="button button-small" href=${href('/dataset', { name: d.name })}>Edit schedule</a></td>
          </tr>`)}</tbody>
        <//>`}
    </div>
  <//>`;
}
