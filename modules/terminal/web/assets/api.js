export function createAPI({ websocketURL }) {
  return {
    websocketPath(kind, id = '') {
      const suffix = kind === 'local' ? '/local/ws' : `/${kind}/${encodeURIComponent(id)}/ws`;
      return websocketURL(suffix);
    },
  };
}
