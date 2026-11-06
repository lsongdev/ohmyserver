// Parts of a pool shared by the list and its page: state, how it is built,
// its latest scrub, and its devices as small cards, since a nested table gets
// long and says more than a glance needs.
import { html } from '@lsongdev/panel/html.js';
import { Status, Badge } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { bytes, ago, datetime } from '@lsongdev/panel/format.js';

const STALE_DAYS = 35;
const CLASSES = { log: 'Log', l2cache: 'Cache', spare: 'Spare', special: 'Special', dedup: 'Dedup' };

const scanName = f => (f === 'RESILVER' ? 'resilver' : 'scrub');
const capital = s => s[0].toUpperCase() + s.slice(1);
const groupName = v => v.name.replace(/-\d+$/, ''); // raidz1-0 → raidz1
const leaves = v => (v.children?.length ? v.children.flatMap(leaves) : [v]);

export const scanning = p => p.scan?.state === 'SCANNING';

export function State({ state }) {
  const tone = { ONLINE: 'green', AVAIL: 'green', INUSE: 'blue', OFFLINE: 'amber', DEGRADED: 'amber' }[state] || 'red';
  return html`<${Status} tone=${tone} state=${state} label=${state ? state[0] + state.slice(1).toLowerCase() : 'Unknown'} />`;
}

export function Capacity({ pool: p }) {
  return html`<div class=${`meter ${p.capacity >= 90 ? 'danger' : p.capacity >= 80 ? 'warning' : ''}`} role="progressbar" aria-label=${`${p.name} capacity`}
    aria-valuenow=${p.capacity} aria-valuemin="0" aria-valuemax="100"><span style=${`width:${p.capacity}%`}></span></div>`;
}

// layout says how a pool is built: "raidz1 of 6 disks, 1 log, 1 spare".
export function layout(pool) {
  const vdevs = pool.vdevs || [];
  const count = list => `${list.length} ${list.every(l => l.disk) ? 'disk' : 'device'}${list.length === 1 ? '' : 's'}`;
  const data = vdevs.filter(v => !CLASSES[v.class]);
  const parts = data.filter(v => v.children?.length).map(g => `${groupName(g)} of ${count(leaves(g))}`);
  const single = data.filter(v => !v.children?.length);
  if (single.length) parts.push(`${count(single)} without redundancy`);
  const extras = {};
  for (const v of vdevs) if (CLASSES[v.class]) extras[CLASSES[v.class]] = (extras[CLASSES[v.class]] || 0) + 1;
  for (const [kind, n] of Object.entries(extras)) parts.push(`${n} ${kind.toLowerCase()}`);
  return parts.join(', ');
}

// ScanFlags are what about the latest scrub needs attention.
export function ScanFlags({ pool: p }) {
  const s = p.scan;
  if (!p.vdevs) return null;
  if (!s) return html`<${Badge} tone="amber">Never scrubbed<//>`;
  if (s.state === 'CANCELED') return html`<${Badge} tone="amber">Last ${scanName(s.function)} canceled<//>`;
  if (s.state !== 'FINISHED') return null;
  const scrub = s.function === 'SCRUB';
  return html`
    ${s.errors ? html`<${Badge} tone="red">${s.errors} unrepaired<//>` : null}
    ${scrub && (Date.now() / 1000 - s.end) / 86400 > STALE_DAYS ? html`<${Badge} tone="amber">Scrub overdue<//>` : null}
    ${scrub && s.issued * 2 < p.alloc ? html`<${Badge} tone="amber">Most data not yet scrubbed<//>` : null}`;
}

export function ScanLine({ pool: p }) {
  const s = p.scan;
  if (!s) return html`<div class="pool-line"><${ScanFlags} pool=${p} /> <span>Nothing has checked this pool's data against its checksums yet.</span></div>`;
  if (s.state === 'SCANNING') {
    const pct = s.to_examine ? Math.min(100, Math.floor((s.issued / s.to_examine) * 100)) : 0;
    return html`<div class="pool-line">${capital(scanName(s.function))} ${s.paused ? 'paused at' : 'running,'} ${pct}%: ${bytes(s.issued)} of ${bytes(s.to_examine)}${s.errors ? `, ${s.errors} errors` : ''}</div>
      <div class="meter" role="progressbar" aria-label=${`${p.name} ${scanName(s.function)}`} aria-valuenow=${pct} aria-valuemin="0" aria-valuemax="100"><span style=${`width:${pct}%`}></span></div>`;
  }
  const when = s.end || s.start;
  if (s.state === 'CANCELED') return html`<div class="pool-line"><${ScanFlags} pool=${p} /> <span title=${datetime(when)}>${ago(when)}</span></div>`;
  const scrub = s.function === 'SCRUB';
  return html`<div class="pool-line">
    <${ScanFlags} pool=${p} />
    <span>Last ${scanName(s.function)} <span title=${datetime(when)}>${ago(when)}</span> checked ${bytes(s.issued)}${scrub ? ` of the ${bytes(p.alloc)} now allocated` : ''}, repaired ${bytes(s.repaired)}.</span>
  </div>`;
}

function errorsOf(v) {
  return [[v.read_errors, 'read'], [v.write_errors, 'write'], [v.checksum_errors, 'checksum']].filter(([n]) => n > 0).map(([n, kind]) => `${n} ${kind}`);
}

// DeviceCard is one device. It has no SMART data: that belongs to the Disks
// module, which its link opens.
function DeviceCard({ vdev: v }) {
  const { link } = usePanel();
  const href = v.disk && link?.('disks', `/disks/${v.disk}`);
  const errors = errorsOf(v);
  const short = v.disk || v.name.split('/').pop();
  const healthy = v.state === 'ONLINE' || v.state === 'AVAIL' || v.state === 'INUSE';
  const body = html`
    <div class="device-head"><strong>${short}</strong><${State} state=${v.state} /></div>
    <div class="device-id" title=${v.name}>${v.disk ? v.name.replace(/^(ata|nvme|scsi|usb)-/, '') : v.name}</div>
    <div class="device-numbers">
      <span>${v.size ? bytes(v.size) : '—'}</span>
      ${v.class === 'spare' ? null : errors.length ? html`<span class="device-errors">${errors.join(', ')} errors</span>` : html`<span>No errors</span>`}
    </div>`;
  const cls = `device-card${errors.length || !healthy ? ' device-trouble' : ''}`;
  return href ? html`<a class=${cls} href=${href} title=${`Open disk ${v.disk}`}>${body}</a>` : html`<div class=${cls}>${body}</div>`;
}

// DeviceGroups draws a pool's devices, group by group as zpool lists them.
export function DeviceGroups({ pool }) {
  const groups = [];
  const loose = {};
  for (const v of pool.vdevs || []) {
    const label = CLASSES[v.class] || '';
    if (v.children?.length) {
      groups.push({ key: v.name, title: label ? `${label} · ${v.name}` : v.name, vdev: v, devices: leaves(v) });
    } else {
      const name = label || 'Data';
      if (!loose[name]) groups.push(loose[name] = { key: name, title: name, devices: [] });
      loose[name].devices.push(v);
    }
  }
  return groups.map(g => {
    const errors = g.vdev ? errorsOf(g.vdev) : [];
    return html`<div class="device-group" key=${g.key}>
      <div class="device-group-head">
        <h3>${g.title}</h3>
        ${g.vdev ? html`<${State} state=${g.vdev.state} />` : null}
        ${errors.length ? html`<span class="device-errors small">${errors.join(', ')} errors</span>` : null}
      </div>
      <div class="device-grid">${g.devices.map(v => html`<${DeviceCard} key=${v.name} vdev=${v} />`)}</div>
    </div>`;
  });
}
