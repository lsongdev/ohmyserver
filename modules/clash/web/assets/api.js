export function createAPI({ request, apiURL }) {
  return {
    list: (options = {}) => request(`/servers`, options),
    create: value => request(`/servers`, { method: 'POST', body: value }),
    get: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}`, options),
    update: (id, value) => request(`/servers/${encodeURIComponent(id)}`, { method: 'PUT', body: value }),
    remove: id => request(`/servers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    test: id => request(`/servers/${encodeURIComponent(id)}/test`, { method: 'POST' }),
    proxies: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}/proxies`, options),
    rules: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}/rules`, options),
    connections: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}/connections`, options),
    config: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}/config`, options),
    updateConfig: (id, value) => request(`/servers/${encodeURIComponent(id)}/config`, { method: 'PATCH', body: value }),
    action: (id, value) => request(`/servers/${encodeURIComponent(id)}/actions`, { method: 'POST', body: value }),
    eventsURL: id => apiURL(`/servers/${encodeURIComponent(id)}/events`),
  };
}
