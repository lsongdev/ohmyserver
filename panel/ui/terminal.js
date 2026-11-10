// The panel's terminal protocol, for the library's Terminal component: a
// WebSocket carrying the session's bytes both ways, plus a text message
// {"type":"resize","cols":…,"rows":…} whenever the terminal changes size.
// The SSH and Docker modules' terminal endpoints speak it.

/**
 * terminalSocket returns a Terminal `connect` function. url(size) gives the
 * endpoint for the terminal's first size ({ cols, rows }), e.g. from
 * client.socketURL. onOpen runs once connected; onClose(event, term) when
 * the socket closes, for showing why.
 */
export function terminalSocket(url, { onOpen, onClose } = {}) {
  return term => {
    const ws = new WebSocket(url({ cols: term.cols, rows: term.rows }));
    ws.binaryType = 'arraybuffer';
    const encoder = new TextEncoder();
    const send = data => { if (ws.readyState === WebSocket.OPEN) ws.send(data); };
    ws.onopen = () => { onOpen?.(); term.focus(); };
    ws.onmessage = event => term.write(typeof event.data === 'string' ? event.data : new Uint8Array(event.data));
    ws.onclose = event => onClose?.(event, term);
    const input = term.onData(data => send(encoder.encode(data)));
    const resize = term.onResize(({ cols, rows }) => send(JSON.stringify({ type: 'resize', cols, rows })));
    return () => {
      ws.onclose = null; // closing it ourselves is not a session ending
      input.dispose();
      resize.dispose();
      ws.close();
    };
  };
}
