import { html, useLayoutEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, Loading, PageHeader } from '@lsongdev/scripts/react/components/index.js';

const commands = [
  ['Modem info', 'ATI'],
  ['SIM status', 'AT+CPIN?'],
  ['5G registration', 'AT+C5GREG?'],
  ['Serving cell', 'AT+QENG="servingcell"'],
  ['GNSS status', 'AT+QGPS?'],
  ['Start GNSS', 'AT+QGPS=1'],
];

const maxExchanges = 100;

export default function AT() {
  const { api } = usePanel();
  const availability = useResource(signal => api.get('/at', { signal }), []);
  const action = useAction();
  const [command, setCommand] = useState('');
  const [exchanges, setExchanges] = useState([]);
  const output = useRef(null);
  const sequence = useRef(0);

  useLayoutEffect(() => {
    if (output.current) output.current.scrollTop = output.current.scrollHeight;
  }, [exchanges]);

  const run = (value, clearInput = false) => action.run(async () => {
    const id = ++sequence.current;
    setExchanges(previous => [...previous.slice(-(maxExchanges - 1)), { id, command: value }]);
    const finish = result => setExchanges(previous => previous.map(entry => entry.id === id ? { ...entry, ...result } : entry));
    try {
      const result = await api.post('/at', { command: value });
      finish({ result });
      if (clearInput && result.final === 'OK') setCommand('');
    } catch (error) {
      finish({ error: error.fields?.command || error.message });
    }
  });
  const submit = event => {
    event.preventDefault();
    if (command.trim()) return run(command, true);
  };

  return html`
    <${PageHeader} title="AT Commands" />
    <${Alert} error=${availability.error} />
    ${availability.loading ? html`<${Loading} />` : null}
    ${availability.data?.available === false ? html`<${Alert} variant="info">Enable panel sign-in to send AT commands.<//>` : null}
    ${availability.data?.available ? html`
      <section class="terminal-frame cellular-console" aria-label="AT console">
        <div class="log cellular-console-log" role="log" aria-label="Command responses" tabindex="0" ref=${output}>
          ${exchanges.length === 0 ? html`<p class="cellular-console-empty">Enter an AT command or use a shortcut below.</p>` : null}
          ${exchanges.map(entry => html`
            <div key=${entry.id} class="cellular-console-entry">
              <div class="cellular-console-command"><span aria-hidden="true">></span> ${entry.command}</div>
              ${entry.result ? html`
                ${entry.result.lines?.length ? html`<pre class="cellular-command-response">${entry.result.lines.join('\n')}</pre>` : null}
                <div class=${entry.result.final === 'OK' ? 'cellular-console-ok' : 'log-stderr'}>${entry.result.final}</div>
              ` : entry.error ? html`<div class="log-stderr">${entry.error}</div>`
                : html`<div class="cellular-console-empty">Waiting for modem…</div>`}
            </div>
          `)}
        </div>
        <div class="cellular-console-controls">
          <div class="cellular-command-buttons" role="group" aria-label="Quick commands">
            ${commands.map(([label, value]) => html`
              <button key=${value} type="button" class="button" title=${value} disabled=${action.busy}
                onClick=${() => run(value)}>${label}</button>
            `)}
          </div>
          <form class="cellular-console-form" onSubmit=${submit}>
            <span class="cellular-console-prompt" aria-hidden="true">></span>
            <input class="input" aria-label="AT command" placeholder="AT" required maxlength="256"
              autocomplete="off" spellcheck="false" value=${command} readonly=${action.busy}
              onInput=${event => setCommand(event.currentTarget.value)} />
            <button type="submit" class="button button-primary" disabled=${action.busy || !command.trim()}>
              ${action.busy ? 'Sending…' : 'Send'}
            </button>
          </form>
        </div>
      </section>
    ` : null}
  `;
}
