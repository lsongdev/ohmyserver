export function createAPI({ request, fetchImpl = fetch }) {
  async function run(type, serverID, onProgress, { signal } = {}) {
    const response = await fetchImpl(`/run`, { method: 'POST', credentials: 'same-origin', signal, headers: { Accept: 'text/event-stream', 'Content-Type': 'application/json' }, body: JSON.stringify({ type, server_id: serverID }) });
    if (!response.ok) throw new Error((await response.text()) || response.statusText || '测速请求失败');
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
    while (true) {
      const { value, done } = await reader.read(); buffer += decoder.decode(value || new Uint8Array(), { stream: !done }); let boundary;
      while ((boundary = buffer.indexOf('\n\n')) !== -1) { const block = buffer.slice(0, boundary); buffer = buffer.slice(boundary + 2); const line = block.split('\n').find(row => row.startsWith('data:')); if (!line) continue; const payload = JSON.parse(line.slice(5).trim()); onProgress(payload); if (payload.stage === 'error') throw new Error(payload.message || '测速失败'); }
      if (done) return;
    }
  }
  return {
    listResults: (options = {}) => request(`/results`, options),
    deleteResult: id => request(`/results/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    listServers: (type, options = {}) => request(`/servers?type=${encodeURIComponent(type)}`, options),
    run,
  };
}
