// A ZFS pool's health: its state, latest scrub or resilver, and devices.
import { html } from '../lib/html.js';
import { Notice, Status, Badge, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes, ago, datetime } from '../lib/format.js';

const STALE_DAYS = 35;
const CLASSES = { log: 'Log', l2cache: 'Cache', spare: 'Spare', special: 'Special', dedup: 'Dedup' };

const name = f => (f === 'RESILVER' ? 'resilver' : 'scrub');
const capital = s => s[0].toUpperCase() + s.slice(1);

// PoolState: anything but ONLINE, or a spare's AVAIL, deserves a look.
function PoolState({ state }) {
  const tone = { ONLINE: 'green', AVAIL: 'green', INUSE: 'blue', OFFLINE: 'amber', DEGRADED: 'amber' }[state] || 'red';
  return html`<${Status} tone=${tone} state=${state} label=${state ? state[0] + state.slice(1).toLowerCase() : 'Unknown'} />`;
}

function ScanLine({ pool: p }) {
  const s = p.scan;
  if (!s) return html`<p class="pool-scan small"><${Badge} tone="amber">Never scrubbed<//> Nothing has checked this pool's data against its checksums yet.</p>`;
  if (s.state === 'SCANNING') {
    const pct = s.to_examine ? Math.min(100, Math.floor((s.issued / s.to_examine) * 100)) : 0;
    return html`<p class="pool-scan small">${capital(name(s.function))} ${s.paused ? 'paused at' : 'running,'} ${pct}%: ${bytes(s.issued)} of ${bytes(s.to_examine)}${s.errors ? `, ${s.errors} errors` : ''}</p>
      <div class="meter" role="progressbar" aria-label=${`${p.name} ${name(s.function)}`} aria-valuenow=${pct} aria-valuemin="0" aria-valuemax="100"><span style=${`width:${pct}%`}></span></div>`;
  }
  if (s.state === 'CANCELED') {
    return html`<p class="pool-scan small"><${Badge} tone="amber">Canceled<//> The last ${name(s.function)} was canceled <span title=${datetime(s.end || s.start)}>${ago(s.end || s.start)}</span>.</p>`;
  }
  const days = (Date.now() / 1000 - s.end) / 86400;
  const partial = s.function === 'SCRUB' && s.issued * 2 < p.alloc;
  return html`<p class="pool-scan small">
    ${s.errors ? html`<${Badge} tone="red">${s.errors} unrepaired<//> ` : null}
    ${s.function === 'SCRUB' && days > STALE_DAYS ? html`<${Badge} tone="amber">Scrub overdue<//> ` : null}
    ${partial ? html`<${Badge} tone="amber">Most data not yet scrubbed<//> ` : null}
    Last ${name(s.function)} <span title=${datetime(s.end)}>${ago(s.end)}</span> checked ${bytes(s.issued)}${
      s.function === 'SCRUB' ? ` of the ${bytes(p.alloc)} now allocated` : ''}, repaired ${bytes(s.repaired)}.
  </p>`;
}

function Device({ vdev: v, depth, href }) {
  const errors = v.read_errors + v.write_errors + v.checksum_errors;
  const count = n => html`<td class="num">${n ?? '—'}</td>`;
  return html`
    <tr class=${errors ? 'attr-alert' : ''}>
      <td><span class=${`pool-indent-${Math.min(depth, 2)}`}>
        ${v.disk ? html`<a class="cell-main" href=${href(`/disks/${v.disk}`)}>${v.disk}</a><div class="cell-sub">${v.name}</div>`
          : html`<span class="cell-main">${v.name}</span>`}
      </span></td>
      <td>${CLASSES[v.class] ? html`<${Badge}>${CLASSES[v.class]}<//>` : v.children ? v.type : 'Disk'}</td>
      <td><${PoolState} state=${v.state} /></td>
      ${v.class === 'spare' ? html`<td class="num">—</td><td class="num">—</td><td class="num">—</td>` : html`${count(v.read_errors)}${count(v.write_errors)}${count(v.checksum_errors)}`}
    </tr>
    ${(v.children || []).map(c => html`<${Device} key=${c.name} vdev=${c} depth=${depth + 1} href=${href} />`)}`;
}

export function PoolCard({ pool: p }) {
  const { href, link } = usePanel();
  const snapshots = link?.('zfs', '/datasets');
  return html`<div class="card pool-card">
    <div class="array-head">
      <h3>${p.name}</h3>
      <${PoolState} state=${p.state} />
      <span class="muted small">${bytes(p.alloc)} of ${bytes(p.size)} allocated</span>
      ${snapshots ? html`<a class="small" href=${snapshots}>Snapshots and schedules</a>` : null}
    </div>
    ${p.status ? html`<${Notice} tone=${p.state === 'ONLINE' ? 'info' : 'warn'}>${p.status}${p.action ? html` ${p.action}` : null}<//>` : null}
    ${p.errors ? html`<${Notice}>${p.errors} known data error${p.errors === 1 ? '' : 's'}. Run <code>zpool status -v ${p.name}</code> on the server for the affected files.<//>` : null}
    <${ScanLine} pool=${p} />
    <div class="pool-devices">
      <${Table}>
        <thead><tr><th>Device</th><th>Type</th><th>State</th><th class="num">Read</th><th class="num">Write</th><th class="num">Checksum</th></tr></thead>
        <tbody>${p.vdevs.map(v => html`<${Device} key=${v.name} vdev=${v} depth=${0} href=${href} />`)}</tbody>
      <//>
    </div>
  </div>`;
}
