
export function createAPI({ request } = {}) {
  return {
    getSettings: ({ signal } = {}) => request(`/settings`, { signal }),
    getStatus: ({ signal } = {}) => request(`/status`, { signal }),
    updateSettings: (settings) => request(`/settings`, { method: 'PUT', body: settings }),
    action: (action) => request(`/action`, { method: 'POST', body: { action } }),
    listLeases: ({ signal } = {}) => request(`/leases`, { signal }),
    listStaticLeases: ({ signal } = {}) => request(`/static-leases`, { signal }),
    createStaticLease: (lease) => request(`/static-leases`, { method: 'POST', body: lease }),
    updateStaticLease: (id, lease) => request(`/static-leases/${id}`, { method: 'PUT', body: lease }),
    deleteStaticLease: (id) => request(`/static-leases/${id}`, { method: 'DELETE' }),
    deleteLease: (id) => request(`/leases/${id}`, { method: 'DELETE' }),
  };
}
