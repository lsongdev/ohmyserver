import { html, render, useEffect, useRef, useState } from 'https://lsong.org/scripts/react/index.js';
import { Terminal } from 'https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/+esm';
import { FitAddon } from 'https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/+esm';

function App() {
  const host = useRef(null);
  const [status, setStatus] = useState('连接中…');
  const [error, setError] = useState('');
  useEffect(() => {
    const terminal = new Terminal({ cursorBlink: true, fontSize: 13, convertEol: true, fontFamily: 'monospace' });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host.current);
    fit.fit();
    const apiBase = document.querySelector('meta[name="api-base"]')?.content || '';
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${scheme}//${location.host}${apiBase}/local/ws?cols=${terminal.cols}&rows=${terminal.rows}`);
    socket.binaryType = 'arraybuffer';
    const input = terminal.onData(data => { if (socket.readyState === WebSocket.OPEN) socket.send(new TextEncoder().encode(data)); });
    const resize = () => {
      fit.fit();
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows }));
    };
    const observer = new ResizeObserver(resize);
    observer.observe(host.current);
    socket.addEventListener('open', () => { setStatus('已连接'); resize(); terminal.focus(); });
    socket.addEventListener('message', event => {
      if (typeof event.data === 'string') {
        try { const message = JSON.parse(event.data); if (message.type === 'error') { setError(message.message); setStatus('连接失败'); } }
        catch { terminal.write(event.data); }
      } else terminal.write(new Uint8Array(event.data));
    });
    socket.addEventListener('close', () => setStatus('连接已关闭'));
    socket.addEventListener('error', () => { setError('无法连接到本机终端'); setStatus('连接失败'); });
    const onResize = () => resize();
    window.addEventListener('resize', onResize);
    return () => { window.removeEventListener('resize', onResize); observer.disconnect(); input.dispose(); socket.close(); terminal.dispose(); };
  }, []);
  return html`<section class="terminal-page card"><div class="page-toolbar"><div><h1>本机终端</h1><span role="status">${status}</span></div></div>${error && html`<div class="notice-card error" role="alert">${error}</div>`}<div class="terminal-viewport" ref=${host}></div></section>`;
}

render(html`<${App} />`, document.getElementById('app'));
