import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, Badge, Empty, Field, Loading, PageHeader, toast } from '@lsongdev/scripts/react/components/index.js';
import { Dialog } from '@lsongdev/scripts/react/components/dialog.js';

const storageName = name => ({ SM: 'SIM (SM)', ME: 'Modem (ME)', MT: 'Combined (MT)' })[name] || name;

export default function SMS() {
  const { api, beforeLeave } = usePanel();
  const availability = useResource(signal => api.get('/sms', { signal }), []);
  const available = availability.data?.available === true;
  const inbox = useResource(signal => available ? api.post('/sms/list', undefined, { signal }) : null, [available]);
  const readiness = useResource(signal => available ? api.get('/sms/check', { signal }) : null, [available]);
  const send = useAction();
  const [open, setOpen] = useState(false);
  const [number, setNumber] = useState('');
  const [text, setText] = useState('');
  const dirty = Boolean(number || text);

  useEffect(() => {
    if (!dirty && !send.busy) return;
    const unregister = beforeLeave(() => window.confirm(
      send.busy ? 'An SMS submission is in progress. Leave this page?' : 'Leave this page and discard the SMS draft?',
    ));
    const warn = event => { event.preventDefault(); event.returnValue = ''; };
    window.addEventListener('beforeunload', warn);
    return () => {
      unregister();
      window.removeEventListener('beforeunload', warn);
    };
  }, [beforeLeave, dirty, send.busy]);

  const submit = event => {
    event.preventDefault();
    if (inbox.loading) return;
    return send.run(async () => {
      let result;
      try {
        result = await api.post('/sms/send', { number, text });
      } finally {
        readiness.reload();
      }
      setNumber('');
      setText('');
      setOpen(false);
      toast(result.parts === 1 ? 'SMS accepted by the modem' : `${result.parts} SMS parts accepted by the modem`, 'success');
    });
  };

  const actions = available ? html`
    <button class="button" type="button" disabled=${inbox.loading || send.busy} onClick=${() => { inbox.reload(); readiness.reload(); }}>Refresh</button>
    <button class="button button-primary" type="button" onClick=${() => setOpen(true)}>Send SMS</button>
  ` : null;

  return html`
    <${PageHeader} title="SMS" actions=${actions} />
    <${Alert} error=${availability.error || (available && inbox.error)} />
    ${availability.loading || (available && inbox.loading && !inbox.data) ? html`<${Loading} />` : null}
    ${availability.data?.available === false ? html`<${Alert} variant="info">Enable panel sign-in to view and send SMS.<//>` : null}
    ${available && readiness.error ? html`<${Alert} error=${readiness.error} />` : null}
    ${available && readiness.data && !readiness.data.ready ? html`
      <${Alert} variant="warning">${readiness.data.issues.join(' ')}<//>
    ` : null}
    ${available && inbox.data ? html`
      <section class="card" aria-label="Messages">
        <p class="muted">${inbox.data.messages.length} messages · ${inbox.data.stores.map(store => `${storageName(store.name)}: ${store.error ? 'unavailable' : `${store.records} records`}`).join(' · ')}</p>
        ${inbox.data.stores.filter(store => store.error).map(store => html`
          <${Alert} key=${store.name} variant="warning">${storageName(store.name)}: ${store.error}<//>
        `)}
        ${inbox.data.messages.length === 0 ? html`<${Empty}>${inbox.data.stores.every(store => store.error) ? 'Messages could not be loaded.' : 'No messages in these stores.'}<//>` : html`
          <ul class="list">
            ${inbox.data.messages.toReversed().map(message => html`
              <li key=${message.id} class="list-item">
                <div class="list-item-content">
                  <div class="sms-headline">
                    <strong class="list-item-headline">${message.number || 'Message'}</strong>
                    ${[...new Set(message.sources.map(source => source.storage))].map(storage => html`<${Badge} key=${storage}>${storageName(storage)}<//>`)}
                  </div>
                  <div class="list-item-supporting">
                    ${[message.status, message.time && new Date(message.time).toLocaleString(),
                      message.parts > 1 && (message.missing?.length ? `${message.received} of ${message.parts} parts received` : `${message.parts} parts combined`)].filter(Boolean).join(' · ')}
                  </div>
                  ${message.error ? html`<p class="muted">${message.error}</p>` : null}
                  ${message.text ? html`<p class="sms-text">${message.text}</p>` : null}
                  <details class="small muted">
                    <summary>Storage records</summary>
                    <ul>${message.sources.map(source => html`<li key=${`${source.storage}:${source.index}`}>${storageName(source.storage)} #${source.index}${source.part ? ` · part ${source.part}` : ''}</li>`)}</ul>
                  </details>
                </div>
              </li>
            `)}
          </ul>
        `}
      </section>
    ` : null}
    <${Dialog} open=${open} aria-label="Send SMS" closeOnBackdrop=${!send.busy}
      onClose=${() => setOpen(false)} onCancel=${event => { if (send.busy) event.preventDefault(); }}>
      <header class="dialog-header"><h2>Send SMS</h2></header>
      <form class="dialog-body" onSubmit=${submit}>
        <${Field} label="Recipient" error=${send.error?.fields?.number}>
          <input class="input" type="tel" required maxlength="16" autofocus value=${number}
            disabled=${send.busy} onInput=${event => setNumber(event.currentTarget.value)} />
        <//>
        <${Field} label="Message" error=${send.error?.fields?.text}>
          <textarea class="input" required rows="4" maxlength="2048" value=${text}
            disabled=${send.busy} onInput=${event => setText(event.currentTarget.value)}></textarea>
        <//>
        ${send.error?.code === 'network'
          ? html`<${Alert} variant="warning">Connection lost. The modem may have received the message. Check before sending again.<//>`
          : html`<${Alert} error=${send.error} />`}
        <div class="form-actions dialog-footer">
          <button class="button" type="button" disabled=${send.busy} onClick=${() => setOpen(false)}>Cancel</button>
          <button class="button button-primary" type="submit" disabled=${send.busy || inbox.loading}>
            ${send.busy ? 'Sending…' : 'Send SMS'}
          </button>
        </div>
      </form>
    <//>
  `;
}
