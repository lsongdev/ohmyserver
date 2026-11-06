import { html, useState } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Empty, Table, Status, toast, toastError } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { ago, datetime } from '@lsongdev/panel/format.js';
import { channelKind, ruleText, Unprotected } from '../notify.js';

export default function Channels() {
  const { api, href } = usePanel();
  const channels = useResource(signal => api.get('/channels', { signal }), [], { interval: 15000 });
  const [testing, setTesting] = useState(null);
  const test = async c => {
    setTesting(c.id);
    try {
      await api.post(`/channels/${c.id}/test`);
      toast(`Test message sent to ${c.name}`, 'success');
    } catch (err) {
      toastError(err);
    } finally {
      setTesting(null);
      channels.reload();
    }
  };
  return html`<${Page} title="Channels" subtitle="Where events are sent, and which ones"
    actions=${html`<span class="notify-actions">
      <a class="button" href=${href('/deliveries')}>Delivery log</a>
      <a class="button button-primary" href=${href('/channels/new')}>Add channel</a>
    </span>`}>
    <${Unprotected} />
    <${Notice} error=${channels.error} />
    ${!channels.data ? (channels.error ? null : html`<${Loading} />`) : channels.data.length === 0 ? html`<${Empty}>No channels yet: events stay here. Add one to be told when something goes wrong.<//>` : html`
      <${Table}>
        <thead><tr><th>Channel</th><th>Sends</th><th>Status</th><th class="actions"></th></tr></thead>
        <tbody>${channels.data.map(c => html`<tr key=${c.id}>
          <td>
            <a class="cell-main" href=${href(`/channels/${c.id}`)}>${c.name}</a>
            <div class="cell-sub">${channelKind(c)} · ${c.target}</div>
          </td>
          <td>${ruleText(c)}</td>
          <td>
            ${!c.enabled ? html`<${Status} state="stopped" label="Off" />`
              : c.last_error ? html`<${Status} state="failed" label="Failing" />`
              : c.last_success_at ? html`<${Status} state="ok" label="Working" />`
              : html`<${Status} state="waiting" tone="blue" label="Not used yet" />`}
            ${c.last_error ? html`<div class="cell-sub notify-body">${c.last_error}</div>`
              : c.last_success_at ? html`<div class="cell-sub" title=${datetime(c.last_success_at)}>Last sent ${ago(c.last_success_at)}</div>` : null}
          </td>
          <td class="actions"><button class="button button-small" disabled=${testing === c.id} onClick=${() => test(c)}>${testing === c.id ? 'Sending…' : 'Send test'}</button></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
