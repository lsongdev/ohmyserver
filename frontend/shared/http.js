export class APIError extends Error {
  constructor(message, status, code) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.code = code;
  }
}

export function createRequest({ fetchImpl = fetch } = {}) {
  return async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    let body = options.body;
    if (body != null && !(body instanceof FormData) && typeof body !== 'string') {
      headers.set('Content-Type', 'application/json');
      body = JSON.stringify(body);
    }
    headers.set('Accept', 'application/json');
    const response = await fetchImpl(path, { ...options, body, headers, credentials: 'same-origin' });
    if (response.status === 204) return null;
    const type = response.headers.get('content-type') || '';
    const payload = type.includes('application/json') ? await response.json() : null;
    if (!response.ok) {
      const error = payload?.error || {};
      throw new APIError(error.message || response.statusText, response.status, error.code);
    }
    return payload?.data ?? payload;
  };
}

export function createModuleTransport({ apiBase, request = createRequest(), fetchImpl = fetch, origin = globalThis.location?.origin || 'http://localhost' }) {
  const moduleRoot = new URL(`${apiBase.replace(/\/$/, '')}/`, origin);
  const resolve = path => {
    if (typeof path !== 'string' || /^[a-z][a-z0-9+.-]*:/i.test(path) || path.startsWith('//')) throw new TypeError('module request path must be relative');
    const url = new URL(path.replace(/^\/+/, ''), moduleRoot);
    if (url.origin !== moduleRoot.origin || !url.pathname.startsWith(moduleRoot.pathname)) throw new TypeError('module request escaped its API mount');
    return url;
  };
  const scopedFetch = (path, options = {}) => fetchImpl(resolve(path).href, { credentials: 'same-origin', ...options });
  return {
    request: (path, options) => request(resolve(path).href, options),
    fetchImpl: scopedFetch,
    apiURL: path => resolve(path).href,
    websocketURL: path => {
      const url = resolve(path);
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      return url.href;
    },
  };
}
