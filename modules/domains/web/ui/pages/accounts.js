import { html } from '../lib/html.js';
import { useBusy } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Status, confirmDanger, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { ago } from '../lib/format.js';
import { useApi } from '../domains.js';

export default function Accounts() {
  const { api, href } = usePanel();
  const accounts = useApi('/accounts');
  const [busy, run] = useBusy(toastError);
  const remove = a => {
    if (!confirmDanger(`Remove the account ${a.name}? Its domains stay at ${a.title}; they just stop appearing here, and certificates for them fall back to manual DNS-01.`)) return;
    run(a.id, async () => { await api.del(`/accounts/${a.id}`); accounts.reload(); });
  };
  return html`<${Page} title="Accounts" subtitle="DNS provider accounts. Their credentials stay on this server and are never shown again."
    actions=${html`<a class="button button-primary" href=${href('/accounts/new')}>Add account</a>`}>
    <${Notice} error=${accounts.error} />
    ${accounts.data === undefined ? (accounts.error ? null : html`<${Loading} />`) : accounts.data.length === 0 ? html`<${Empty}>No accounts yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Account</th><th>Provider</th><th>Status</th><th>Added</th><th class="actions"></th></tr></thead>
        <tbody>${accounts.data.map(a => html`<tr key=${a.id}>
          <td><a class="cell-main" href=${href(`/accounts/${a.id}`)}>${a.name}</a></td>
          <td>${a.title || a.kind}</td>
          <td>${a.error ? html`<${Status} state="failed" label="Failing" title=${a.error} />`
            : a.zones >= 0 ? html`<${Status} state="ok" label=${`${a.zones} ${a.zones === 1 ? 'domain' : 'domains'}`} />` : html`<span class="muted">—</span>`}</td>
          <td class="nowrap muted">${ago(a.created_at)}</td>
          <td class="actions"><span class="button-row">
            <a class="button button-small" href=${href(`/accounts/${a.id}`)}>Edit</a>
            <button class="button button-small button-danger" disabled=${busy[a.id]} onClick=${() => remove(a)}>Remove</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
