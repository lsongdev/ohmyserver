
export function createAPI({ request } = {}) {
  return {
    list: (view = 'installed', q = '', options = {}) => request(`/packages?view=${encodeURIComponent(view)}&q=${encodeURIComponent(q)}`, options),
    refresh: () => request(`/refresh`, { method: 'POST' }),
    install: name => request(`/install`, { method: 'POST', body: { name } }),
    remove: name => request(`/remove`, { method: 'POST', body: { name } }),
    upgrade: (name = '') => request(`/upgrade`, { method: 'POST', body: { name } }),
  };
}
