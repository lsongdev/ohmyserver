import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { PageHeader, Alert, Loading, toast } from '@lsongdev/scripts/react/components/index.js';

export default function Subscription({ params }) {
  const { api } = usePanel();
  const resource = useResource(signal => api.get('/subscriptions/' + params.id, { signal }), [params.id]);
  if (resource.error) return html`<${Alert} error=${resource.error} />`;
  if (!resource.data) return html`<${Loading} />`;
  return html`<${Detail} key=${resource.data.revision} data=${resource.data} reload=${resource.reload} />`;
}

function Detail({ data, reload }) {
  const { api, href } = usePanel();
  const sub = data.subscription;
  const candidates = sub.candidates || [];
  const [selected, setSelected] = useState([]);
  const { busy, error, run } = useAction();
  const saved = useUnsavedChanges(selected.length > 0, { pending: busy });
  const refresh = () => {
    if (selected.length && !window.confirm('Fetch again and clear the current selection?')) return;
    run(async () => {
      await api.post('/subscriptions/' + sub.id + '/refresh', { revision: data.revision });
      saved();
      reload();
    });
  };
  const submit = e => {
    e.preventDefault();
    run(async () => {
      await api.post('/subscriptions/' + sub.id + '/import', { revision: data.revision, ids: selected });
      toast('Outbounds saved to draft. Apply them from Overview.', 'success');
      saved();
      reload();
    });
  };
  return html`<${PageHeader} title=${sub.name} subtitle=${sub.updated ? 'Last fetched ' + new Date(sub.updated).toLocaleString() : 'Fetch the subscription to preview nodes.'}
    actions=${html`<a class="button" href=${href('/outbounds/subscriptions/' + sub.id + '/edit')}>Edit</a>
      <button class="button button-primary" disabled=${busy} onClick=${refresh}>${busy ? 'Working…' : 'Fetch subscription'}</button>`} />
    <${Alert} error=${error || sub.error} />
    ${sub.warnings?.length > 0 && html`<${Alert} variant="warning"><ul>${sub.warnings.map(w => html`<li>${w}</li>`)}</ul><//>`}
    ${sub.nodes.some(node => node.missing) && html`<${Alert} variant="warning">
      These imported nodes are missing from the latest subscription and remain in the saved configuration.
      <ul>${sub.nodes.filter(node => node.missing).map(node => html`<li>${node.name} · ${node.tag}</li>`)}</ul>
    <//>`}
    ${candidates.length > 0 && html`<form class="form" onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${busy}>
        <label class="field-check"><input type="checkbox" checked=${selected.length === candidates.length}
          onChange=${e => setSelected(e.currentTarget.checked ? candidates.map(node => node.id) : [])} />Select all (${candidates.length})</label>
        <ul class="xray-rows">${candidates.map(node => {
          const imported = sub.nodes.find(entry => entry.id === node.id);
          return html`<li class="xray-row" key=${node.id}>
            <label class="field-check"><input type="checkbox" checked=${selected.includes(node.id)}
              onChange=${e => setSelected(e.currentTarget.checked ? [...selected, node.id] : selected.filter(id => id !== node.id))} />
              <span>${node.name}<small>${node.protocol} · ${node.address || 'Xray outbound'}${imported ? ' · Imported' : ''}</small></span>
            </label>
          </li>`;
        })}</ul>
        <div class="form-actions">
          <button class="button button-primary" type="submit" disabled=${busy || selected.length === 0}>Import selected (${selected.length})</button>
          <a class="button" href=${href('/outbounds')}>Outbounds</a>
        </div>
      </fieldset>
    </form>`}`;
}
