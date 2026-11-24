import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { PageHeader, Alert, Field, Loading, Dialog, toast } from '@lsongdev/scripts/react/components/index.js';

export default function SubscriptionForm({ params }) {
  const { api } = usePanel();
  const resource = useResource(signal => api.get(params.id ? '/subscriptions/' + params.id : '/subscriptions', { signal }), [params.id]);
  if (resource.error) return html`<${Alert} error=${resource.error} />`;
  if (!resource.data) return html`<${Loading} />`;
  return html`<${Form} data=${resource.data} id=${params.id} />`;
}

function Form({ data, id }) {
  const { api, href, navigate } = usePanel();
  const [name, setName] = useState(data.subscription?.name || '');
  const [url, setURL] = useState('');
  const { busy, error, run } = useAction();
  const saved = useUnsavedChanges(name !== (data.subscription?.name || '') || url !== '', { pending: busy });
  const back = id ? '/outbounds/subscriptions/' + id : '/outbounds';
  const submit = event => {
    event.preventDefault();
    run(async () => {
      const body = { revision: data.revision, name, url };
      const sub = id ? await api.put('/subscriptions/' + id, body) : await api.post('/subscriptions', body);
      toast('Subscription saved', 'success');
      saved();
      navigate('/outbounds/subscriptions/' + sub.id, { replace: true });
    });
  };
  return html`<${PageHeader} title=${id ? 'Edit subscription' : 'Add subscription'}
    back=${{ href: href(back), label: 'Subscriptions' }} />
    <form class="form settings-form" onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${busy}>
        <${Alert} error=${error} />
        <${Field} label="Name" required><input class="input" required value=${name} onInput=${e => setName(e.currentTarget.value)} /><//>
        <${Field} label="Subscription URL" required=${!id}
          hint=${id ? 'Leave empty to keep the saved link.' : 'HTTP or HTTPS subscription link.'}>
          <input class="input" type="url" required=${!id} autocomplete="off" spellcheck="false"
            value=${url} onInput=${e => setURL(e.currentTarget.value)} />
        <//>
        <div class="form-actions"><button class="button button-primary" type="submit">Save subscription</button>
          <a class="button" href=${href(back)}>Cancel</a></div>
      </fieldset>
    </form>`;
}


export function SubscriptionDialog({ onClose, onSaved }) {
  const { api } = usePanel();
  const [name, setName] = useState('');
  const [url, setURL] = useState('');
  const { busy, error, run } = useAction();
  const dirty = name !== '' || url !== '';
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const close = () => {
    if (busy || (dirty && !window.confirm('Discard your unsaved changes?'))) return;
    saved();
    onClose();
  };
  const submit = event => {
    event.preventDefault();
    run(async () => {
      const current = await api.get('/subscriptions');
      await api.post('/subscriptions', { revision: current.revision, name, url });
      toast('Subscription saved', 'success');
      saved();
      onSaved();
    });
  };
  return html`<${Dialog} open aria-label="Add subscription" closeOnBackdrop=${false}
    onCancel=${event => { event.preventDefault(); close(); }} onClose=${onClose}>
    <header class="dialog-header"><h2>Add subscription</h2>
      <button type="button" class="dialog-close" aria-label="Close" disabled=${busy} onClick=${close}>×</button>
    </header>
    <form onSubmit=${submit}>
      <div class="dialog-body">
        <fieldset class="form-fields" disabled=${busy}>
          <${Alert} error=${error} />
          <${Field} label="Name" required>
            <input class="input" required maxlength="200" autofocus value=${name} onInput=${e => setName(e.currentTarget.value)} />
          <//>
          <${Field} label="Subscription URL" required>
            <input class="input" type="url" required autocomplete="off" spellcheck="false"
              value=${url} onInput=${e => setURL(e.currentTarget.value)} />
          <//>
        </fieldset>
      </div>
      <div class="form-actions dialog-footer">
        <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
        <button class="button button-primary" type="submit" disabled=${busy}>${busy ? 'Saving…' : 'Add subscription'}</button>
      </div>
    </form>
  <//>`;
}
