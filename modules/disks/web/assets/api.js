
export function createAPI({ request } = {}) {
  return {
    usage: (options = {}) => request(`/usage`, options),
    devices: (options = {}) => request(`/devices`, options),
    health: (device, options = {}) => request(`/health?device=${encodeURIComponent(device)}`, options),
  };
}
