
export function createAPI({ request } = {}) {
  const base = `/jobs`;
  return {
    list: () => request(base),
    create: (job) => request(base, { method: 'POST', body: job }),
    update: (id, job) => request(`${base}/${id}`, { method: 'PUT', body: job }),
    remove: (id) => request(`${base}/${id}`, { method: 'DELETE' }),
    enabled: (id, enabled) => request(`${base}/${id}/enabled`, { method: 'POST', body: { enabled } }),
    run: (id) => request(`${base}/${id}/run`, { method: 'POST' }),
    executions: (id) => request(`${base}/${id}/executions`),
  };
}
