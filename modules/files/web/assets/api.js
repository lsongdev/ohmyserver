export function createAPI({ request, apiURL }) {
  return {
    list: (path = '', showHidden = false, options = {}) => request(`/entries?path=${encodeURIComponent(path)}&show_hidden=${showHidden}`, options),
    create: (action, path, name, content = '') => request('/entries', { method: 'POST', body: { action, path, name, content } }),
    upload: (path, file) => { const body = new FormData(); body.set('path', path); body.set('file', file); return request('/upload', { method: 'POST', body }); },
    remove: path => request(`/entries?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),
    download: path => apiURL(`/download?path=${encodeURIComponent(path)}`),
  };
}
