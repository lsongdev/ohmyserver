
export function createAPI({ request } = {}) {
  return {
    getSettings: ({signal}={}) => request(`/settings`,{signal}),
    getStatus: ({signal}={}) => request(`/status`,{signal}),
    updateSettings: settings => request(`/settings`,{method:'PUT',body:settings}),
    action: action => request(`/action`,{method:'POST',body:{action}}),
  };
}
