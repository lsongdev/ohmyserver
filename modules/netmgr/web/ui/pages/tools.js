import { html, useEffect, useRef, useState } from '../lib/html.js';
import { Notice, Field } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { ApiError } from '../lib/client.js';
import { Page } from '../network.js';

const TOOLS = {
  ping: { label: 'Ping', hint: 'Sends four echo requests from this server.' },
  trace: { label: 'Trace route', hint: 'Shows each hop on the way there. Can take up to a minute.' },
  lookup: { label: 'DNS lookup', hint: 'Asks this server\'s DNS servers.' },
  port: { label: 'Check TCP port', hint: 'Tries to connect from this server.' },
};
const RECORDS = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'TXT', 'PTR'];

export default function Tools({ query, setQuery }) {
  const { client } = usePanel();
  const tool = TOOLS[query.tool] ? query.tool : 'ping';
  const [target, setTarget] = useState(query.target || '');
  const [port, setPort] = useState('443');
  const [record, setRecord] = useState('A');
  const [output, setOutput] = useState('');
  const [running, setRunning] = useState(false);
  const [error, setError] = useState(null);
  const controller = useRef(null);
  useEffect(() => () => controller.current?.abort(), []);

  // Output arrives line by line, so it is read from the response directly.
  const run = async e => {
    e.preventDefault();
    if (controller.current) return;
    const abort = new AbortController();
    controller.current = abort;
    setRunning(true);
    setError(null);
    setOutput('');
    try {
      const response = await fetch(client.url('/tools/run'), {
        method: 'POST', signal: abort.signal,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tool, target: target.trim(), port: tool === 'port' ? Number(port) : undefined, record: tool === 'lookup' ? record : undefined }),
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => null);
        throw new ApiError(response.status, payload?.error || { message: response.statusText });
      }
      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        setOutput(text => text + value);
      }
    } catch (err) {
      if (err.name !== 'AbortError') setError(err);
    } finally {
      controller.current = null;
      setRunning(false);
    }
  };

  return html`<${Page} title="Diagnostics" subtitle="Test the network from this server's side">
    <form class="form settings-form" onSubmit=${run}>
      <div class="form-grid">
        <${Field} label="Test" hint=${TOOLS[tool].hint}>
          <select class="input" value=${tool} disabled=${running} onChange=${e => setQuery({ tool: e.currentTarget.value })}>
            ${Object.entries(TOOLS).map(([key, t]) => html`<option key=${key} value=${key}>${t.label}</option>`)}
          </select>
        <//>
        <${Field} label="Host or IP address" required>
          <input class="input mono" required value=${target} placeholder="example.com" autofocus onInput=${e => setTarget(e.currentTarget.value)} />
        <//>
        ${tool === 'port' ? html`<${Field} label="Port" required>
          <input class="input mono" required type="number" min="1" max="65535" value=${port} onInput=${e => setPort(e.currentTarget.value)} />
        <//>` : null}
        ${tool === 'lookup' ? html`<${Field} label="Record type">
          <select class="input" value=${record} onChange=${e => setRecord(e.currentTarget.value)}>
            ${RECORDS.map(r => html`<option key=${r} value=${r}>${r}</option>`)}
          </select>
        <//>` : null}
      </div>
      <div class="button-actions">
        <button class="button button-primary" disabled=${running}>${running ? 'Running…' : 'Run'}</button>
        ${running ? html`<button type="button" class="button" onClick=${() => controller.current?.abort()}>Stop</button>` : null}
      </div>
    </form>
    <${Notice} error=${error} />
    ${output || running ? html`<pre class="log tool-output" aria-live="polite">${output || 'Waiting for output…'}</pre>` : null}
  <//>`;
}
