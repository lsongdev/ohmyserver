import { html, useState } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Empty, Table, toast } from '@lsongdev/scripts/react/components/index.js';
import { Status, toastError } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { ago, datetime } from '@lsongdev/panel/format.js';

const STATES = {
  pending: ['pending', 'Waiting'],
  sent: ['ok', 'Sent'],
  merged: ['ok', 'In a summary'],
  failed: ['failed', 'Failed'],
  dropped: ['stopped', 'Dropped'],
};

export default function Deliveries() {
  const { api, query, setQuery, href } = usePanel();
  const state = query.state || '';
  const list = useResource(signal => api.get('/deliveries', { signal, query: { state } }), [state], { interval: 10000 });
  const [busy, setBusy] = useState(null);
  const retry = async d => {
    setBusy(d.id);
    try {
      await api.post(`/deliveries/${d.id}/retry`);
      toast('Queued to send again', 'success');
      list.reload();
    } catch (err) {
      toastError(err);
    } finally {
      setBusy(null);
    }
  };
  return html`<${PageHeader} title="Delivery log" subtitle="What was sent where, and why not" back=${{ href: href('/channels'), label: 'Channels' }} />
    <${Alert} error=${list.error} />
    <div class="notify-filters">
      <select class="input" aria-label="State" value=${state} onChange=${e => setQuery({ state: e.currentTarget.value })}>
        <option value="">All states</option>
        ${Object.entries(STATES).map(([value, [, title]]) => html`<option value=${value}>${title}</option>`)}
      </select>
    </div>
    ${!list.data ? (list.error ? null : html`<${Loading} />`) : list.data.length === 0 ? html`<${Empty}>Nothing here yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Event</th><th>Channel</th><th>State</th><th>When</th><th class="actions"></th></tr></thead>
        <tbody>${list.data.map(d => {
          const [tone, title] = STATES[d.state] || ['stopped', d.state];
          const at = d.finished_at || d.created_at;
          return html`<tr key=${d.id}>
            <td><span class="notify-title">${d.title}</span><div class="cell-sub">${d.source} · ${d.kind === 'resolved' ? 'resolved' : d.level}</div></td>
            <td><a href=${href(`/channels/${d.channel_id}`)}>${d.channel}</a></td>
            <td>
              <${Status} state=${tone} label=${title} />
              ${d.error ? html`<div class="cell-sub notify-body">${d.error}</div>` : null}
              ${d.state === 'pending' && d.attempts ? html`<div class="cell-sub">${d.attempts} failed so far; next try ${ago(d.next_attempt_at)}</div>` : null}
            </td>
            <td class="nowrap" title=${datetime(at)}>${ago(at)}</td>
            <td class="actions">${d.state === 'failed' ? html`<button class="button button-small" disabled=${busy === d.id} onClick=${() => retry(d)}>Send again</button>` : null}</td>
          </tr>`;
        })}</tbody>
      <//>`}`;
}
