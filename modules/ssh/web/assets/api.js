
export function createAPI({ request, apiURL } = {}) {
  const servers = `/servers`;
  return {
    list: () => request(servers),
    create: server => request(servers, { method: 'POST', body: server }),
    get: id => request(`${servers}/${id}`),
    update: (id, server) => request(`${servers}/${id}`, { method: 'PUT', body: server }),
    remove: id => request(`${servers}/${id}`, { method: 'DELETE' }),
    test: server => request(`${servers}/test`, { method: 'POST', body: server }),
    statsURL: id => apiURL(`/servers/${id}/stats`),
    metricsURL: id => apiURL(`/servers/${id}/metrics`),
  };
}
