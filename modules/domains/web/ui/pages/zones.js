import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Badge } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi } from '../domains.js';

export default function Zones() {
  const { api, href, query, setQuery } = usePanel();
  const zones = useApi('/zones');
  const accounts = useApi('/accounts');
  const refresh = useAction();
  const reload = () => refresh.run(async () => { await api.get('/zones', { query: { refresh: 1 } }); zones.reload(); accounts.reload(); });
  const failing = (accounts.data || []).filter(a => a.error);
  const text = (query.q || '').trim().toLowerCase();
  const list = (zones.data || []).filter(z => !text || z.name.includes(text));

  return html`<${Page} title="Domains" subtitle="Zones hosted by your DNS provider accounts."
    actions=${html`<span class="page-actions">
      <button class="button" disabled=${refresh.busy} onClick=${reload}>${refresh.busy ? 'Refreshing…' : 'Refresh'}</button>
      <a class="button button-primary" href=${href('/accounts/new')}>Add account</a></span>`}>
    <${Notice} error=${zones.error || refresh.error} />
    ${failing.map(a => html`<${Notice} key=${a.id} tone="warn">${a.name}: ${a.error}. <a href=${href(`/accounts/${a.id}`)}>Check the account</a>.<//>`)}
    ${accounts.data && accounts.data.length === 0 ? html`<${Empty}>
      No DNS provider accounts yet. <a href=${href('/accounts/new')}>Add one</a> (Cloudflare) to manage its domains here and to issue
      certificates for them, wildcards included, without opening a port.<//>` : html`
      <div class="toolbar">
        <input class="input" type="search" placeholder="Filter domains" aria-label="Filter" value=${query.q || ''}
          onInput=${e => setQuery({ q: e.currentTarget.value })} />
      </div>
      ${zones.data === undefined ? (zones.error ? null : html`<${Loading} />`) : list.length === 0 ? html`<${Empty}>No domains.<//>` : html`
        <${Table}>
          <thead><tr><th>Domain</th><th>Account</th><th>Status</th><th>Name servers</th></tr></thead>
          <tbody>${list.map(z => html`<tr key=${z.account + z.id}>
            <td><a class="cell-main" href=${href(`/zones/${z.account}/${encodeURIComponent(z.id)}`)}>${z.name}</a></td>
            <td>${z.account_name}</td>
            <td>${z.status ? html`<${Badge} tone=${z.status === 'active' ? 'green' : 'amber'}>${z.status}<//>` : '—'}</td>
            <td class="mono small">${(z.name_servers || []).join(', ') || '—'}</td>
          </tr>`)}</tbody>
        <//>`}`}
  <//>`;
}
