import { useEffect, useRef, useState } from 'preact/hooks';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { html } from '@panel/shared/ui.js';

const labels = { local: '本机终端', ssh: 'SSH 终端', docker: '容器终端' };

export default function TerminalSession({ api, kind, params = {}, navigate }) {
  const host = useRef(null), termRef = useRef(null), fitRef = useRef(null), socketRef = useRef(null);
  const [status, setStatus] = useState('正在连接…'), [error, setError] = useState(''), [sessionVersion, setSessionVersion] = useState(0);
  const id = params.id || '';

  useEffect(() => {
    const term = new Terminal({ cursorBlink: true, fontSize: 13, fontFamily: 'Menlo, Monaco, Consolas, monospace', convertEol: true, allowProposedApi: false });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    termRef.current = term;
    fitRef.current = fit;
    fit.fit();
    const socket = new WebSocket(api.websocketPath(kind, id));
    socket.binaryType = 'arraybuffer';
    socketRef.current = socket;
    const sendSize = () => {
      if (socket.readyState !== WebSocket.OPEN) return;
      fit.fit();
      socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    };
    const input = term.onData(data => { if (socket.readyState === WebSocket.OPEN) socket.send(new TextEncoder().encode(data)); });
    const resizeObserver = new ResizeObserver(sendSize);
    resizeObserver.observe(host.current);
    socket.addEventListener('open', () => { setStatus('已连接'); sendSize(); term.focus(); });
    socket.addEventListener('message', event => {
      if (typeof event.data === 'string') {
        try { const message = JSON.parse(event.data); if (message.type === 'error') { setError(message.message || '终端连接失败'); setStatus('连接失败'); } }
        catch { term.write(event.data); }
      } else term.write(new Uint8Array(event.data));
    });
    socket.addEventListener('close', () => setStatus('连接已关闭'));
    socket.addEventListener('error', () => { setError('无法连接到终端服务'); setStatus('连接失败'); });
    const resize = () => sendSize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      resizeObserver.disconnect();
      input.dispose();
      socket.close();
      term.dispose();
      socketRef.current = null;
      termRef.current = null;
      fitRef.current = null;
    };
  }, [kind, id, sessionVersion]);

  const clear = () => termRef.current?.clear();
  const reconnect = () => { setError(''); setStatus('正在重新连接…'); setSessionVersion(value => value + 1); };
  const title = labels[kind] || '终端';
  return html`<section class="terminal-page card">
    <div class="page-toolbar"><div><h2>${title}${id ? ` · ${id}` : ''}</h2><span class="muted" role="status">${status}</span></div>
      <div class="button-group"><button class="button" onClick=${clear}>清屏</button><button class="button" onClick=${reconnect}>重新连接</button>${kind !== 'local' ? html`<button class="button" onClick=${() => navigate(kind === 'ssh' ? '/ssh/servers' : '/docker/containers')}>返回</button>` : null}</div>
    </div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <div class="terminal-viewport" ref=${host} onClick=${() => termRef.current?.focus()}></div>
  </section>`;
}
