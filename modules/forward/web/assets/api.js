
export function createAPI({ request } = {}) {
  return {
    list: ({ signal } = {}) => request(`/listeners`, { signal }),
    create: (item) => request(`/listeners`, { method: 'POST', body: item }),
    update: (id, item) => request(`/listeners/${id}`, { method: 'PUT', body: item }),
    remove: (id) => request(`/listeners/${id}`, { method: 'DELETE' }),
    action: (id, action) => request(`/listeners/${id}/actions/${action}`, { method: 'POST' }),
  };
}
