import { html } from '@lsongdev/panel/html.js';
import { Page, Notice, Loading, Empty, Badge, Status, Table } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { bytes } from '@lsongdev/panel/format.js';
import { useApi, kind, uptime, usage, zfsPools, HealthStatus } from '../disks.js';

export default function Overview() {
  const { href, link } = usePanel();
  const system = useApi('/system');
  const overview = useApi(system.data?.supported ? '/overview' : null, 30000);
  if (system.data && !system.data.supported) {
    return html`<${Page} title="Overview"><${Notice} tone="info">Disk information needs Linux (lsblk) or macOS (diskutil); neither was found.<//><//>`;
  }
  const o = overview.data;
  const install = link?.('packages', '/', { tab: 'search', s: 'smartmontools' });
  return html`<${Page} title="Overview">
    ${system.data && !system.data.smartctl ? html`<${Notice} tone="info">
      Disk health needs smartmontools.${install ? html` <a href=${install}>Install it from Packages</a>.` : ' Install the smartmontools package.'}
    <//>` : null}
    <${Notice} error=${overview.error} />
    ${!o ? (overview.error ? null : html`<${Loading}>Reading disks…<//>`) : html`
      <section class="section">
        <h2>File systems</h2>
        ${o.filesystems.length === 0 ? html`<${Empty}>No mounted file systems.<//>` : html`
          <div class="grid">${o.filesystems.map(fs => html`<${FileSystem} key=${fs.source} fs=${fs} />`)}</div>`}
      </section>
      ${o.arrays.length ? html`<section class="section">
        <h2>RAID arrays</h2>
        <div class="grid-2">${o.arrays.map(a => html`<${ArrayCard} key=${a.name} array=${a} />`)}</div>
      </section>` : null}
      <section class="section">
        <h2>Disks</h2>
        ${o.disks.length === 0 ? html`<${Empty}>No disks found.<//>` : html`
          <${Table}>
            <thead><tr><th>Disk</th><th class="num">Size</th><th>Type</th><th>Health</th><th class="num">Temp</th><th class="num">Powered on</th><th>Used by</th></tr></thead>
            <tbody>${o.disks.map(d => html`<tr key=${d.name}>
              <td><a class="cell-main" href=${href(`/disks/${d.name}`)}>${d.name}</a>
                <div class="cell-sub">${d.model || '—'}</div></td>
              <td class="num">${bytes(d.size)}</td>
              <td>${kind(d)}</td>
              <td><${HealthStatus} health=${d.health} /></td>
              <td class="num">${d.health?.temperature ? `${d.health.temperature} °C` : '—'}</td>
              <td class="num">${uptime(d.health?.power_on_hours)}</td>
              <td class="muted"><${UsedBy} disk=${d} /></td>
            </tr>`)}</tbody>
          <//>`}
      </section>`}
  <//>`;
}

// UsedBy lists what a disk serves; the ZFS pool it belongs to opens there.
function UsedBy({ disk }) {
  const { link } = usePanel();
  const used = usage(disk);
  const pools = zfsPools(disk);
  if (!used.length && !pools.length) return '—';
  return html`${used.join(', ')}${used.length && pools.length ? ', ' : ''}${pools.map((name, i) => {
    const pool = link?.('zfs', '/pool', { name, from: 'disks' });
    return html`${i ? ', ' : ''}${pool ? html`<a href=${pool}>${name} (ZFS)</a>` : `${name} (ZFS)`}`;
  })}`;
}

// FileSystem is a card; a ZFS one is a dataset, and opens in the ZFS module.
function FileSystem({ fs }) {
  const { link } = usePanel();
  const pct = fs.size ? Math.round((fs.used / fs.size) * 100) : 0;
  const dataset = fs.type === 'zfs' && link?.('zfs', '/dataset', { name: fs.source, from: 'disks' });
  const body = html`
    <div class="fs-head"><strong>${fs.mountpoint}</strong><span class="muted small">${fs.type}</span></div>
    <div class="cell-sub">${fs.source}</div>
    <div class=${`meter ${pct >= 90 ? 'danger' : pct >= 80 ? 'warning' : ''}`}><span style=${`width:${pct}%`}></span></div>
    <div class="fs-numbers"><span>${pct}% used · ${bytes(fs.used)}</span><span>${bytes(fs.available)} free of ${bytes(fs.size)}</span></div>`;
  return dataset
    ? html`<a class="card fs-card" href=${dataset} title=${`Open dataset ${fs.source}`}>${body}</a>`
    : html`<div class="card fs-card">${body}</div>`;
}

function ArrayCard({ array: a }) {
  let state;
  if (a.state !== 'active') state = html`<${Status} state="failed" label=${a.state} />`;
  else if (a.sync) state = html`<${Status} state="processing" label=${`${a.sync.action[0].toUpperCase()}${a.sync.action.slice(1)}${a.sync.progress ? ` ${a.sync.progress}%` : ''}`} />`;
  else if (a.degraded) state = html`<${Status} state="failed" label=${`Degraded (${a.working} of ${a.disks})`} />`;
  else state = html`<${Status} state="ok" label="Healthy" />`;
  return html`<div class="card">
    <div class="array-head"><h3>${a.name}</h3>${state}<span class="muted small">${a.level} · ${bytes(a.size)}${a.read_only ? ' · read-only' : ''}</span></div>
    ${a.sync?.progress ? html`<div class="meter"><span style=${`width:${a.sync.progress}%`}></span></div>
      <div class="muted small" style="margin-top:6px">${a.sync.finish ? `About ${a.sync.finish.replace('min', ' min')} left` : ''}${a.sync.speed ? ` at ${a.sync.speed}` : ''}</div>` : null}
    <div class="members">${a.members.map(m => html`<${Badge} key=${m.name} tone=${m.faulty ? 'red' : m.spare ? undefined : 'green'}
      title=${m.faulty ? 'Faulty' : m.spare ? 'Spare' : `Slot ${m.role}`}>${m.name}${m.faulty ? ' · faulty' : m.spare ? ' · spare' : ''}<//>`)}</div>
  </div>`;
}
