import { html, useEffect, useRef, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Status, confirmDanger } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { target } from './hosts.js';

// Close codes the API uses besides the standard ones.
const CONNECT_FAILED = 4000;
const HOST_KEY_CHANGED = 4001;

const LABELS = { connecting: 'Connecting…', open: 'Connected', closed: 'Session ended', error: 'The terminal failed to load' };

export default function Terminal({ params }) {
  const { api, client, href } = usePanel();
  const host = useResource(signal => api.get(`/hosts/${params.id}`, { signal }), [params.id]);
  const ref = useRef();
  const [status, setStatus] = useState('connecting');
  const [closed, setClosed] = useState(null); // { code, reason }
  const [session, setSession] = useState(0);
  const forget = useAction();
  const ready = host.data !== undefined;

  useEffect(() => {
    if (!ready) return;
    let disposed = false;
    let cleanup = () => {};
    setClosed(null);
    // xterm is large, so it is loaded only when a terminal opens. Inside a
    // panel the module lives in a shadow root, which document styles do not
    // reach: add the stylesheet where the terminal renders.
    const root = ref.current.getRootNode();
    const where = root instanceof ShadowRoot ? root : document.head;
    if (!root.querySelector('link[data-xterm]')) {
      const link = Object.assign(document.createElement('link'), { rel: 'stylesheet', href: new URL('../node_modules/@xterm/xterm/css/xterm.css', import.meta.url).href });
      link.dataset.xterm = '';
      where.append(link);
    }
    Promise.all([import('../node_modules/@xterm/xterm/lib/xterm.mjs'), import('../node_modules/@xterm/addon-fit/lib/addon-fit.mjs')]).then(([{ Terminal: XTerm }, { FitAddon }]) => {
      if (disposed) return;
      const term = new XTerm({ cursorBlink: true, fontFamily: 'ui-monospace, Menlo, Consolas, monospace', fontSize: 13, theme: { background: '#000000' } });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(ref.current);
      fit.fit();
      setStatus('connecting');
      term.write(`\x1b[90mConnecting to ${target(host.data)}…\x1b[0m\r\n`);
      const ws = new WebSocket(client.socketURL(`hosts/${params.id}/terminal`, { cols: term.cols, rows: term.rows }));
      ws.binaryType = 'arraybuffer';
      const encoder = new TextEncoder();
      ws.onopen = () => { setStatus('open'); term.focus(); };
      ws.onmessage = e => term.write(new Uint8Array(e.data));
      ws.onclose = e => {
        setStatus('closed');
        setClosed({ code: e.code, reason: e.reason });
        const text = e.code === HOST_KEY_CHANGED ? 'The host key changed; connection refused' : e.reason || 'Connection closed';
        term.write(`\r\n\x1b[90m[${text}]\x1b[0m\r\n`);
      };
      const input = term.onData(data => ws.readyState === 1 && ws.send(encoder.encode(data)));
      const resize = term.onResize(({ cols, rows }) => ws.readyState === 1 && ws.send(JSON.stringify({ type: 'resize', cols, rows })));
      const observer = new ResizeObserver(() => fit.fit());
      observer.observe(ref.current);
      cleanup = () => { observer.disconnect(); input.dispose(); resize.dispose(); ws.close(); term.dispose(); };
    }).catch(() => setStatus('error'));
    return () => { disposed = true; cleanup(); };
  }, [params.id, session, ready]);

  const forgetKey = () => {
    if (!confirmDanger('Continue only if you know the server was reinstalled or its key replaced: forget the recorded host key and accept the one the server offers now?')) return;
    forget.run(async () => {
      await api.del(`/hosts/${params.id}/host-key`);
      setSession(s => s + 1);
    });
  };

  if (!ready) return html`<${Page} title="Terminal">${host.error ? html`<${Notice} error=${host.error} />` : html`<${Loading} />`}<//>`;
  const h = host.data;
  return html`<${Page} title=${h.name} subtitle=${html`<a href=${href('/')}>← Hosts</a> · <span class="mono">${target(h)}</span>`}
    actions=${html`
      <${Status} state=${status === 'open' ? 'running' : status === 'connecting' ? 'starting' : 'stopped'} label=${LABELS[status]} />
      ${status === 'closed' ? html`<button class="button button-small" onClick=${() => setSession(s => s + 1)}>Reconnect</button>` : null}`}>
    ${closed?.code === HOST_KEY_CHANGED ? html`<${Notice} tone="warn">
      The host key the server offers (<span class="mono">${closed.reason}</span>) differs from the one recorded on the first connection. The server may have been reinstalled, or someone may be intercepting the connection.
      <div style="margin-top:8px"><button class="button button-small button-danger" disabled=${forget.busy} onClick=${forgetKey}>Forget the key and reconnect</button></div>
    <//>` : null}
    ${closed?.code === CONNECT_FAILED ? html`<${Notice}>Connection failed: ${closed.reason}<//>` : null}
    <${Notice} error=${forget.error} />
    <div class="terminal" ref=${ref}></div>
  <//>`;
}
