
export function createAPI({ request } = {}) {
  return {
    list: ({ signal } = {}) => request(`/`, { signal }),
    create: (setting) => request(`/`, { method: 'POST', body: setting }),
    update: (id, value) => request(`/${id}`, { method: 'PUT', body: { value } }),
    delete: (id) => request(`/${id}`, { method: 'DELETE' }),
  };
}
