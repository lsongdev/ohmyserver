import { html, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Terminal as TerminalView } from '@lsongdev/scripts/react/components/index.js';
import { terminalSocket } from '@lsongdev/panel/terminal.js';
import { Status } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { target } from './hosts.js';

// Close codes the API uses besides the standard ones.
const CONNECT_FAILED = 4000;
const HOST_KEY_CHANGED = 4001;

// xterm is large, so it loads only when a terminal opens.
const XTERM_CSS = new URL('../node_modules/@xterm/xterm/css/xterm.css', import.meta.url).href;
const loadXterm = () => Promise.all([
  import('../node_modules/@xterm/xterm/lib/xterm.mjs'),
  import('../node_modules/@xterm/addon-fit/lib/addon-fit.mjs'),
]).then(([{ Terminal }, { FitAddon }]) => ({ Terminal, FitAddon }));

const LABELS = { connecting: 'Connecting…', open: 'Connected', closed: 'Session ended', error: 'The terminal failed to load' };

export default function Terminal({ params }) {
  const { api, client, href } = usePanel();
  const host = useResource(signal => api.get(`/hosts/${params.id}`, { signal }), [params.id]);
  const [status, setStatus] = useState('connecting');
  const [closed, setClosed] = useState(null); // { code, reason }
  const [session, setSession] = useState(0);
  const forget = useAction();
  const ready = host.data !== undefined;

  // Each session is a fresh Terminal (keyed by session), reconnecting.
  const connect = terminalSocket(size => client.socketURL(`hosts/${params.id}/terminal`, size), {
    onOpen: () => setStatus('open'),
    onClose: (e, term) => {
      setStatus('closed');
      setClosed({ code: e.code, reason: e.reason });
      const text = e.code === HOST_KEY_CHANGED ? 'The host key changed; connection refused' : e.reason || 'Connection closed';
      term.write(`\r\n\x1b[90m[${text}]\x1b[0m\r\n`);
    },
  });
  const start = term => {
    setStatus('connecting');
    setClosed(null);
    term.write(`\x1b[90mConnecting to ${target(host.data)}…\x1b[0m\r\n`);
    return connect(term);
  };

  const forgetKey = () => {
    if (!window.confirm('Continue only if you know the server was reinstalled or its key replaced: forget the recorded host key and accept the one the server offers now?')) return;
    forget.run(async () => {
      await api.del(`/hosts/${params.id}/host-key`);
      setSession(s => s + 1);
    });
  };

  if (!ready) return html`<${PageHeader} title="Terminal" />${host.error ? html`<${Alert} error=${host.error} />` : html`<${Loading} />`}`;
  const h = host.data;
  return html`<${PageHeader} title=${h.name} back=${{ href: href('/'), label: 'Hosts' }} subtitle=${html`<span class="mono">${target(h)}</span>`}
    actions=${html`
      <${Status} state=${status === 'open' ? 'running' : status === 'connecting' ? 'starting' : 'stopped'} label=${LABELS[status]} />
      ${status === 'closed' ? html`<button class="button button-small" onClick=${() => setSession(s => s + 1)}>Reconnect</button>` : null}`} />
    ${closed?.code === HOST_KEY_CHANGED ? html`<${Alert} variant="warning">
      The host key the server offers (<span class="mono">${closed.reason}</span>) differs from the one recorded on the first connection. The server may have been reinstalled, or someone may be intercepting the connection.
      <div style="margin-top:8px"><button class="button button-small button-danger" disabled=${forget.busy} onClick=${forgetKey}>Forget the key and reconnect</button></div>
    <//>` : null}
    ${closed?.code === CONNECT_FAILED ? html`<${Alert} variant="danger">Connection failed: ${closed.reason}<//>` : null}
    <${Alert} error=${forget.error} />
    <${TerminalView} key=${`${params.id}:${session}`} load=${loadXterm} css=${XTERM_CSS} connect=${start} onError=${() => setStatus('error')} />`;
}
