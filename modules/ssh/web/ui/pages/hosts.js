import { html } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Empty, Badge, confirmDanger, Table } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { routeURL } from '@lsongdev/panel/forms.js';
import { ago } from '@lsongdev/panel/format.js';

export const AUTH = { password: 'Password', key: 'Private key', panel: 'Panel key' };

export const target = h => `${h.user}@${h.address}${h.port === 22 ? '' : `:${h.port}`}`;

export default function Hosts() {
  const { api, href, query } = usePanel();
  const list = useResource(signal => api.get('/hosts', { signal }), []);
  const action = useAction();

  const remove = h => {
    if (!confirmDanger(`Delete the host ${h.name}?`)) return;
    action.run(async () => { await api.del(`/hosts/${h.id}`); list.reload(); });
  };

  return html`<${Page} title="Hosts" subtitle="Save SSH hosts and open terminals to them in the browser."
    actions=${html`<a class="button button-primary" href=${href('/hosts/new', { return: routeURL('/', query) })}>Add host</a>`}>
    <${Notice} error=${list.error || action.error} />
    ${list.data === undefined ? (list.error ? null : html`<${Loading} />`) : !list.data.length ? html`<${Empty}>No hosts yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Address</th><th>Authentication</th><th>Host key</th><th>Last connected</th><th class="actions"></th></tr></thead>
        <tbody>${list.data.map(h => html`<tr key=${h.id}>
          <td><div class="cell-main">${h.name}</div>${h.description ? html`<div class="cell-sub">${h.description}</div>` : null}</td>
          <td class="mono">${target(h)}</td>
          <td><${Badge}>${AUTH[h.auth] || h.auth}<//></td>
          <td>${h.host_key_sha256
            ? html`<span class="mono cell-sub" title=${`${h.host_key_type} ${h.host_key_sha256}`}>${h.host_key_sha256.slice(0, 19)}…</span>`
            : html`<span class="muted">Recorded on first connection</span>`}</td>
          <td class="nowrap muted">${h.last_connected_at ? ago(h.last_connected_at) : '—'}</td>
          <td class="actions"><span class="row-actions">
            <a class="button button-small button-primary" href=${href(`/hosts/${h.id}/terminal`)}>Terminal</a>
            <a class="button button-small" href=${href(`/hosts/${h.id}/edit`, { return: routeURL('/', query) })}>Edit</a>
            <button class="button button-small button-danger" disabled=${action.busy} onClick=${() => remove(h)}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
