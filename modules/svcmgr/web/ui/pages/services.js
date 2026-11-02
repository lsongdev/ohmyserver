import { html } from '../lib/html.js';
import { useBusy } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Tabs, Table, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, ServiceStatus, Unsupported, isActive, unitLabel } from '../services.js';

const FILTERS = {
  all: () => true,
  running: u => u.active === 'active' && u.sub === 'running',
  failed: u => u.active === 'failed',
  enabled: u => u.enabled === 'enabled',
};

export default function Services() {
  const { api, href, query, setQuery } = usePanel();
  const system = useApi('/system');
  const list = useApi(system.data?.systemd ? '/services' : null, 10000);
  const [busy, run] = useBusy(toastError);

  if (system.data && !system.data.systemd) return html`<${Page} title="Services"><${Unsupported} /><//>`;
  const units = list.data || [];
  const show = query.show || 'all';
  const text = (query.q || '').trim().toLowerCase();
  const matches = units.filter(FILTERS[show] || FILTERS.all)
    .filter(u => !text || u.name.toLowerCase().includes(text) || u.description?.toLowerCase().includes(text));
  const count = f => units.filter(FILTERS[f]).length;
  const self = system.data?.self;

  const act = (u, action) => {
    if (action !== 'start' && !window.confirm(`${action[0].toUpperCase() + action.slice(1)} ${unitLabel(u.name)}?`)) return;
    run(u.name, async () => { await api.post(`/services/${encodeURIComponent(u.name)}/${action}`); list.reload(); });
  };

  return html`<${Page} title="Services" subtitle=${system.data?.version ? `systemd ${system.data.version}` : null}>
    <${Notice} error=${list.error} />
    <${Tabs} active=${show} onChange=${id => setQuery({ show: id === 'all' ? '' : id })} tabs=${[
      ['all', `All (${units.length})`], ['running', `Running (${count('running')})`],
      ['failed', `Failed (${count('failed')})`], ['enabled', `Enabled (${count('enabled')})`]]} />
    <div class="toolbar">
      <input class="input" type="search" placeholder="Filter by name or description" aria-label="Filter"
        value=${query.q || ''} onInput=${e => setQuery({ q: e.currentTarget.value })} />
    </div>
    ${!list.data ? (list.error ? null : html`<${Loading} />`) : matches.length === 0 ? html`<${Empty}>No service matches.<//>` : html`
      <${Table}>
        <thead><tr><th>Service</th><th>Status</th><th>Start at boot</th><th class="actions"></th></tr></thead>
        <tbody>${matches.map(u => html`<tr key=${u.name}>
          <td>
            <a class="cell-main" href=${href(`/services/${encodeURIComponent(u.name)}`)}>${unitLabel(u.name)}</a>
            ${u.name === self ? html` <span class="muted small">(this panel)</span>` : null}
            ${u.description ? html`<div class="cell-sub truncate" style="max-width:440px" title=${u.description}>${u.description}</div>` : null}
          </td>
          <td><${ServiceStatus} unit=${u} /></td>
          <td class="muted">${u.enabled || '—'}</td>
          <td class="actions"><span class="row-actions">
            ${isActive(u) ? html`
              <button class="button button-small" disabled=${busy[u.name]} onClick=${() => act(u, 'restart')}>Restart</button>
              ${u.name === self ? null : html`<button class="button button-small" disabled=${busy[u.name]} onClick=${() => act(u, 'stop')}>Stop</button>`}`
            : u.load === 'loaded' && (u.enabled === 'enabled' || u.enabled === 'disabled' || u.active === 'failed') ? html`<button class="button button-small" disabled=${busy[u.name]} onClick=${() => act(u, 'start')}>Start</button>` : null}
          </span></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
