export function createAPI({ request }) {
  return {
    hosts: options => request(`/hosts`, options),
    history: (hostname, duration, options) => request(`/hosts/${encodeURIComponent(hostname)}/history?duration=${encodeURIComponent(duration)}`, options),
    current: options => request(`/current`, options),
  };
}
