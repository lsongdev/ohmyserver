export function createAPI({ request } = {}) {
  return {
    list: ({ signal } = {}) => request(`/widgets`, { signal }),
    create: (item) => request(`/widgets`, { method:'POST', body:item }),
    update: (id, item) => request(`/widgets/${id}`, { method:'PUT', body:item }),
    remove: (id) => request(`/widgets/${id}`, { method:'DELETE' }),
  };
}
