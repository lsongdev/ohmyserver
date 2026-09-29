
export function createAPI({ request } = {}) {
  return {
    list: ({ signal } = {}) => request(`/entries`, { signal }),
    create: (setting) => request(`/entries`, { method: 'POST', body: setting }),
    update: (id, value) => request(`/entries/${encodeURIComponent(id)}`, { method: 'PUT', body: { value } }),
    delete: (id) => request(`/entries/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  };
}
