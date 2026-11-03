import { html, useMemo, useState } from '../lib/html.js';
import { Page, Notice, Loading, Empty } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { routeURL } from '../lib/forms.js';
import { useApi } from '../zfs.js';
import { SnapshotTable, TakeSnapshotDialog } from '../components/snapshots.js';

// Snapshots across every dataset, to search by name or kind. A dataset's own
// are on its page too; a link here with ?dataset= filters to them.
export default function Snapshots() {
  const { query, setQuery } = usePanel();
  const dataset = query.dataset || '';
  const search = query.q || '';
  const kind = query.kind || '';
  const datasets = useApi('/datasets');
  const snapshots = useApi('/snapshots', { dataset });
  const [taking, setTaking] = useState(false);

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return (snapshots.data || []).filter(s =>
      (!needle || s.name.toLowerCase().includes(needle)) &&
      (!kind || (kind === 'manual' ? !s.class : s.class === kind)));
  }, [snapshots.data, search, kind]);
  const changed = () => { snapshots.reload(); datasets.reload(); };

  return html`<${Page} title="Snapshots" subtitle="Read-only points in time of each dataset; they share space until the data diverges"
    actions=${html`<button class="button button-primary" disabled=${!datasets.data?.length} onClick=${() => setTaking(true)}>Take snapshot</button>`}>
    <${Notice} error=${snapshots.error || datasets.error} />
    <div class="zfs-filters">
      <select class="input" aria-label="Dataset" value=${dataset} onChange=${e => setQuery({ dataset: e.currentTarget.value })}>
        <option value="">All datasets</option>
        ${(datasets.data || []).map(d => html`<option value=${d.name}>${d.name}</option>`)}
      </select>
      <select class="input" aria-label="Kind" value=${kind} onChange=${e => setQuery({ kind: e.currentTarget.value })}>
        <option value="">All kinds</option>
        <option value="manual">Manual</option>
        ${['hourly', 'daily', 'weekly', 'monthly'].map(c => html`<option value=${c}>Auto · ${c}</option>`)}
      </select>
      <input class="input" type="search" placeholder="Filter by name" aria-label="Filter by name" value=${search}
        onInput=${e => setQuery({ q: e.currentTarget.value })} />
    </div>
    ${!snapshots.data ? (snapshots.error ? null : html`<${Loading} />`) : visible.length === 0 ? html`<${Empty}>${
      snapshots.data.length ? 'No snapshot matches the filter.' : 'No snapshots yet. Take one now, or set a schedule on a dataset.'
    }<//>` : html`<${SnapshotTable} snapshots=${visible} returnTo=${routeURL('/snapshots', query)} onChanged=${changed} />`}
    ${taking ? html`<${TakeSnapshotDialog} datasets=${datasets.data} dataset=${dataset} onClose=${() => setTaking(false)} onCreated=${changed} />` : null}
  <//>`;
}
