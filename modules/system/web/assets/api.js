
export function createAPI({ request }) {
  return {
    getInfo: options => request(`/info`, options),
  };
}
