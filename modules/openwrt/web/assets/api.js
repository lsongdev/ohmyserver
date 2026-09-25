
export function createAPI({ request }) {
  const path = id => `/routers/${encodeURIComponent(id)}`;
  return {
    listRouters: options => request(`/routers`, options),
    getRouter: (id, options) => request(path(id), options),
    createRouter: data => request(`/routers`, { method: 'POST', body: data }),
    updateRouter: (id, data) => request(path(id), { method: 'PUT', body: data }),
    deleteRouter: id => request(path(id), { method: 'DELETE' }),
    testRouter: id => request(`${path(id)}/test`, { method: 'POST' }),
    readSection: (id, section, options) => request(`${path(id)}/${section}`, options),
    action: (id, data) => request(`${path(id)}/actions`, { method: 'POST', body: data }),
  };
}
