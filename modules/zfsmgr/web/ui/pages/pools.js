import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Badge } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes } from '../lib/format.js';
import { useApi } from '../zfs.js';
import { State, Capacity, ScanFlags, layout, scanning } from '../components/pool.js';

const REFRESH = 15000;
const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;

function PoolCard({ pool: p, datasets }) {
  const { href } = usePanel();
  const own = datasets.filter(d => d.name === p.name || d.name.startsWith(p.name + '/'));
  const snapshots = own.reduce((sum, d) => sum + d.snapshots, 0);
  const built = layout(p);
  return html`<a class="card pool-card pool-link" href=${href('/pool', { name: p.name })}>
    <div class="pool-head">
      <h2>${p.name}</h2>
      <${State} state=${p.health} />
      <span class="muted small pool-capacity">${bytes(p.alloc)} of ${bytes(p.size)} allocated (${p.capacity}%)</span>
    </div>
    <${Capacity} pool=${p} />
    <div class="pool-line">
      <span>${[built, plural(own.length, 'dataset'), plural(snapshots, 'snapshot')].filter(Boolean).join(' · ')}</span>
      ${scanning(p) ? html`<${Badge} tone="blue">${p.scan.function === 'RESILVER' ? 'Resilvering' : 'Scrubbing'}<//>` : null}
      <${ScanFlags} pool=${p} />
      ${p.errors ? html`<${Badge} tone="red">${p.errors} data errors<//>` : null}
    </div>
  </a>`;
}

export default function Pools() {
  const { href } = usePanel();
  const list = useApi('/pools', undefined, REFRESH);
  const datasets = useApi('/datasets');
  const data = list.data;
  return html`<${Page} title="ZFS pools" subtitle="Each pool combines disks into one space, and holds the datasets that keep your files"
    actions=${html`<a class="button button-primary" href=${href('/pool/new')}>Create pool</a>`}>
    <${Notice} error=${list.error || datasets.error} />
    ${data?.details_error ? html`<${Notice} tone="warn">${data.details_error}<//>` : null}
    ${!data ? (list.error ? null : html`<${Loading} />`)
      : data.pools.length === 0 ? html`<${Empty}>No pools on this server.<//>`
      : data.pools.map(p => html`<${PoolCard} key=${p.name} pool=${p} datasets=${datasets.data || []} />`)}
  <//>`;
}
