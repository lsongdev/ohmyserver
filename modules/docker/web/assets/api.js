
export function createAPI({ request, apiURL, fetchImpl = fetch }) {
  async function streamContainerStats(id, onStats, { signal } = {}) {
    const response = await fetchImpl(`/containers/${encodeURIComponent(id)}/stats`, { signal, credentials: 'same-origin', headers: { Accept: 'text/event-stream' } });
    if (!response.ok) { const payload = await response.json().catch(() => null); throw new Error(payload?.error?.message || response.statusText || '容器统计读取失败'); }
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
    while (true) {
      const { value, done } = await reader.read(); buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
      let boundary;
      while ((boundary = buffer.indexOf('\n\n')) !== -1) {
        const block = buffer.slice(0, boundary); buffer = buffer.slice(boundary + 2);
        const event = block.match(/^event:\s*(.+)$/m)?.[1]; const data = block.match(/^data:\s*(.+)$/m)?.[1];
        if (!event || !data) continue; const payload = JSON.parse(data);
        if (event === 'stats') onStats(payload);
        if (event === 'error') throw new Error(payload.message || '容器统计读取失败');
      }
      if (done) return;
    }
  }
  async function pullImage(name, { onProgress = () => {}, signal } = {}) {
    const response = await fetchImpl(`/images/pull`, {
      method: 'POST', credentials: 'same-origin', signal,
      headers: { Accept: 'text/event-stream, application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => null);
      throw new Error(payload?.error?.message || response.statusText || '镜像拉取失败');
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
      let boundary;
      while ((boundary = buffer.indexOf('\n\n')) !== -1) {
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const event = block.match(/^event:\s*(.+)$/m)?.[1];
        const data = block.match(/^data:\s*(.+)$/m)?.[1];
        if (!event || !data) continue;
        const payload = JSON.parse(data);
        if (event === 'progress') onProgress(payload.message);
        if (event === 'error') throw new Error(payload.message || '镜像拉取失败');
        if (event === 'done') return;
      }
      if (done) return;
    }
  }
  async function buildImage(data, { onLog = () => {}, signal } = {}) {
    const response = await fetchImpl(`/images/build`, { method: 'POST', credentials: 'same-origin', signal, headers: { Accept: 'text/event-stream, application/json', 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    if (!response.ok) { const payload = await response.json().catch(() => null); throw new Error(payload?.error?.message || response.statusText || '镜像构建失败'); }
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
    while (true) {
      const { value, done } = await reader.read(); buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
      let boundary;
      while ((boundary = buffer.indexOf('\n\n')) !== -1) {
        const block = buffer.slice(0, boundary); buffer = buffer.slice(boundary + 2);
        const event = block.match(/^event:\s*(.+)$/m)?.[1]; const payload = block.match(/^data:\s*(.+)$/m)?.[1];
        if (!event || !payload) continue; const message = JSON.parse(payload).message;
        if (event === 'log') onLog(message);
        if (event === 'error') throw new Error(message || '镜像构建失败');
        if (event === 'done') return;
      }
      if (done) return;
    }
  }
  return {
    listContainers: (options = {}) => request(`/containers`, options),
    createContainer: data => request(`/containers`, { method: 'POST', body: data }),
    getContainer: (id, options = {}) => request(`/containers/${encodeURIComponent(id)}`, options),
    getContainerLogs: (id, options = {}) => request(`/containers/${encodeURIComponent(id)}/logs`, options),
    streamContainerStats,
    startContainer: id => request(`/containers/${encodeURIComponent(id)}/start`, { method: 'POST' }),
    stopContainer: id => request(`/containers/${encodeURIComponent(id)}/stop`, { method: 'POST' }),
    restartContainer: id => request(`/containers/${encodeURIComponent(id)}/restart`, { method: 'POST' }),
    deleteContainer: id => request(`/containers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    listImages: () => request(`/images`),
    pullImage,
    buildImage,
    deleteImage: id => request(`/images/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    listVolumes: () => request(`/volumes`),
    createVolume: data => request(`/volumes`, { method: 'POST', body: data }),
    deleteVolume: name => request(`/volumes/${encodeURIComponent(name)}`, { method: 'DELETE' }),
    listNetworks: () => request(`/networks`),
    createNetwork: data => request(`/networks`, { method: 'POST', body: data }),
    deleteNetwork: id => request(`/networks/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    listComposeProjects: (options = {}) => request(`/compose`, options),
    createComposeProject: data => request(`/compose`, { method: 'POST', body: data }),
    getComposeProject: (name, options = {}) => request(`/compose/${encodeURIComponent(name)}`, options),
    composeAction: (name, action) => request(`/compose/${encodeURIComponent(name)}/${encodeURIComponent(action)}`, { method: 'POST' }),
    listApps: ({ filter = '', category = '', ...options } = {}) => { const query = new URLSearchParams(); if (filter) query.set('filter', filter); if (category) query.set('category', category); const suffix = query.toString(); return request(`/apps${suffix ? `?${suffix}` : ''}`, options); },
    appCategories: (options = {}) => request(`/apps/categories`, options),
    syncApps: () => request(`/apps/sync`, { method: 'POST' }),
    getApp: (id, options = {}) => request(`/apps/${encodeURIComponent(id)}`, options),
    appIconURL: id => apiURL(`/apps/${encodeURIComponent(id)}/icon`),
    installApp: (id, environment) => request(`/apps/${encodeURIComponent(id)}/install`, { method: 'POST', body: { environment } }),
    appInstallStatus: (id, options = {}) => request(`/apps/${encodeURIComponent(id)}/install`, options),
    appAction: (id, action) => request(`/apps/${encodeURIComponent(id)}/${encodeURIComponent(action)}`, { method: 'POST' }),
  };
}
