
export function createAPI({ request, apiURL }) {
  const project = id => `/projects/${encodeURIComponent(id)}`;
  const deployment = id => `/deployments/${encodeURIComponent(id)}`;

  return {
    list: options => request(`/projects`, options),
    get: (id, options) => request(project(id), options),
    create: value => request(`/projects`, { method: 'POST', body: value }),
    update: (id, value) => request(project(id), { method: 'PUT', body: value }),
    remove: id => request(project(id), { method: 'DELETE' }),
    deployments: (id, options) => request(`${project(id)}/deployments`, options),
    start: id => request(`${project(id)}/deployments`, { method: 'POST' }),
    deployment: (id, options) => request(deployment(id), options),
    logs: (id, options) => request(`${deployment(id)}/logs`, options),
    cancel: (projectID, deploymentID) => request(`${project(projectID)}/deployments/${encodeURIComponent(deploymentID)}/cancel`, { method: 'POST' }),
    runtime: (id, options) => request(`${project(id)}/runtime`, options),
    rotateSecret: id => request(`${project(id)}/webhook-secret`, { method: 'POST' }),
    webhookURL: id => apiURL(`/hooks/github/${encodeURIComponent(id)}`),
    eventsURL: (projectID, deploymentID, lastLogID = 0) => apiURL(`${project(projectID)}/deployments/${encodeURIComponent(deploymentID)}/events?lastLogID=${encodeURIComponent(lastLogID)}`),
  };
}
