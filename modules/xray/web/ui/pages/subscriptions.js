import { html, useRef, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, Loading, toast } from '@lsongdev/scripts/react/components/index.js';
import { SubscriptionDialog } from './subscription-form.js';

export default function Subscriptions({ onChange = () => {} }) {
  const { api, href, query } = usePanel();
  const list = useResource(signal => api.get('/subscriptions', { signal }), []);
  const [adding, setAdding] = useState(query.addSubscription === '1');
  const addButton = useRef(null);
  const closeDialog = () => {
    setAdding(false);
    requestAnimationFrame(() => addButton.current?.focus());
  };
  const { busy, error, run } = useAction();
  const remove = sub => {
    if (!window.confirm('Remove this subscription? Imported outbounds will remain as manual entries.')) return;
    run(async () => {
      const current = await api.get('/subscriptions');
      await api.post('/subscriptions/' + sub.id + '/delete', { revision: current.revision });
      toast('Subscription removed', 'success');
      list.reload();
      onChange();
    });
  };
  return html`<section class="xray-subscriptions" aria-labelledby="xray-subscriptions-title">
    <div class="xray-summary-heading">
      <h2 id="xray-subscriptions-title">Subscriptions</h2>
      <button ref=${addButton} type="button" class="button button-primary" onClick=${() => setAdding(true)}>Add subscription</button>
    </div>
    <${Alert} error=${error || list.error} />
    ${!list.data ? html`<${Loading} />` : list.data.subscriptions.length === 0
      ? html`<p>No subscriptions yet.</p>`
      : html`<ul class="xray-rows">${list.data.subscriptions.map(sub => html`
        <li class="xray-row" key=${sub.id}>
          <div><a href=${href('/outbounds/subscriptions/' + sub.id)}>${sub.name}</a>
            <small>${sub.nodes.length} imported · ${sub.updated ? 'Fetched ' + new Date(sub.updated).toLocaleString() : 'Not fetched yet'}</small>
            ${sub.error && html`<small>${sub.error}</small>`}
          </div>
          <button type="button" class="button" disabled=${busy} onClick=${() => remove(sub)}>Remove</button>
        </li>`)}</ul>`}
    ${adding && html`<${SubscriptionDialog} onClose=${closeDialog}
      onSaved=${() => { closeDialog(); list.reload(); onChange(); }} />`}
  </section>`;
}
