
export function createAPI({ request } = {}) {
  return {
    list: ({ signal } = {}) => request(`/monitors`, { signal }),
    save: (monitor) => request(`/monitors`, { method: 'POST', body: monitor }),
    update: (id, monitor) => request(`/monitors/${id}`, { method: 'PUT', body: monitor }),
    remove: (id) => request(`/monitors/${id}`, { method: 'DELETE' }),
    action: (id, action) => request(`/monitors/${id}/actions/${action}`, { method: 'POST' }),
    checks: (id, { signal } = {}) => request(`/monitors/${id}/checks?limit=30`, { signal }),
  };
}
