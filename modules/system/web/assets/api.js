
export function createAPI({ request }) {
  return {
    getInfo: (check, options) => request(`/info${check ? '?check=1' : ''}`, options),
    update: () => request(`/update`, { method: 'POST' }),
  };
}
