export function createAPI({ request }) {
  return {
    listServers: (options = {}) => request(`/servers`, options),
    createServer: value => request(`/servers`, { method: 'POST', body: value }),
    getServer: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}`, options),
    updateServer: (id, value) => request(`/servers/${encodeURIComponent(id)}`, { method: 'PUT', body: value }),
    deleteServer: id => request(`/servers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    testServer: id => request(`/servers/${encodeURIComponent(id)}/test`, { method: 'POST' }),
    inventory: (id, options = {}) => request(`/servers/${encodeURIComponent(id)}/inventory`, options),
    power: (id, action) => request(`/servers/${encodeURIComponent(id)}/power`, { method: 'POST', body: { action } }),
  };
}
