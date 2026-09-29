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
    if (!payload || !Object.hasOwn(payload, 'data')) throw new APIError('Invalid JSON response envelope', response.status, 'invalid_response');
    return payload.data;
  };
}

export function createModuleTransport({ apiBase, fetchImpl = fetch, request = createRequest({ fetchImpl }), origin = globalThis.location?.origin || 'http://localhost' }) {
  const root = moduleRoot(apiBase, origin);
  const resolve = path => {
    localPath(path);
    const url = new URL(path.slice(1), root);
    if (url.origin !== root.origin || !url.pathname.startsWith(root.pathname)) throw new TypeError('module request escaped its API mount');
    return url;
  };
  const scopedRequest = (path, options = {}) => request(resolve(path).href, { ...options, credentials: 'same-origin' });
  const scopedFetch = (path, options = {}) => fetchImpl(resolve(path).href, { ...options, credentials: 'same-origin' });
  return Object.freeze({
    request: scopedRequest,
    fetchImpl: scopedFetch,
    apiURL: path => resolve(path).href,
    websocketURL: path => {
      const url = resolve(path);
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      return url.href;
    },
  });
}
import { localPath, moduleRoot } from './paths.js';
