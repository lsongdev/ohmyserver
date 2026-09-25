import { html, render, useEffect, useState } from '@panel/shared/ui.js';
import runtime from './module.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '/v1/dhcp';
const moduleRoot = new URL(`${apiBase.replace(/\/$/, '')}/`, location.origin);

function resolveAPI(path) {
  if (typeof path !== 'string' || /^[a-z][a-z0-9+.-]*:/i.test(path) || path.startsWith('//')) throw new TypeError('module request path must be relative');
  const url = new URL(path.replace(/^\/+/, ''), moduleRoot);
  if (url.origin !== moduleRoot.origin || !url.pathname.startsWith(moduleRoot.pathname)) throw new TypeError('module request escaped its API mount');
  return url;
}

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  let body = options.body;
  if (body != null && !(body instanceof FormData) && typeof body !== 'string') {
    headers.set('Content-Type', 'application/json');
    body = JSON.stringify(body);
  }
  headers.set('Accept', 'application/json');
  const response = await fetch(resolveAPI(path), { ...options, body, headers, credentials: 'same-origin' });
  if (response.status === 204) return null;
  const type = response.headers.get('content-type') || '';
  const payload = type.includes('application/json') ? await response.json() : null;
  if (!response.ok) throw new Error(payload?.error?.message || response.statusText || `HTTP ${response.status}`);
  return payload?.data ?? payload;
}

const transport = {
  request,
  fetchImpl: (path, options = {}) => fetch(resolveAPI(path), { credentials: 'same-origin', ...options }),
  apiURL: path => resolveAPI(path).href,
  websocketURL: path => {
    const url = resolveAPI(path);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.href;
  },
};
const api = runtime.createAPI(transport);

function href(path) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) throw new TypeError('module navigation path must start with a single /');
  return `#${path}`;
}

function navigate(path, { replace = false } = {}) {
  const target = href(path);
  if (replace) {
    history.replaceState(null, '', target);
    dispatchEvent(new Event('hashchange'));
  } else {
    location.hash = target.slice(1);
  }
}

function readLocation() {
  const source = location.hash.startsWith('#') ? location.hash.slice(1) : location.hash;
  const url = new URL(source || '/', location.origin);
  return { pathname: url.pathname, search: url.searchParams };
}

function matchRoute(route, pathname) {
  const expected = route.split('/').filter(Boolean);
  const actual = pathname.split('/').filter(Boolean);
  if (expected.length !== actual.length) return null;
  const params = {};
  for (let i = 0; i < expected.length; i += 1) {
    let segment;
    try { segment = decodeURIComponent(actual[i]); } catch { return null; }
    if (expected[i].startsWith(':')) params[expected[i].slice(1)] = segment;
    else if (expected[i] !== segment) return null;
  }
  return params;
}

function findRoute(pathname) {
  let best = null;
  let score = -1;
  for (const route of runtime.routes) {
    const params = matchRoute(route.path, pathname);
    if (params === null) continue;
    const next = route.path.split('/').filter(Boolean).filter(segment => !segment.startsWith(':')).length;
    if (next > score) { best = { route, params }; score = next; }
  }
  return best;
}

function App() {
  const [manifest, setManifest] = useState(null);
  const [manifestError, setManifestError] = useState('');
  const [locationKey, setLocationKey] = useState(location.hash);
  const [loaded, setLoaded] = useState(null);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    let active = true;
    fetch(new URL('manifest.json', moduleRoot), { credentials: 'same-origin', headers: { Accept: 'application/json' } })
      .then(response => response.ok ? response.json() : Promise.reject(new Error(`模块清单加载失败 (${response.status})`)))
      .then(value => { if (active) setManifest(value); })
      .catch(error => { if (active) setManifestError(error.message || '模块清单加载失败'); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    const update = () => setLocationKey(location.hash);
    addEventListener('hashchange', update);
    return () => removeEventListener('hashchange', update);
  }, []);

  const current = readLocation();
  const pathname = current.pathname === '/' ? (manifest?.home || '/') : current.pathname;
  const matched = findRoute(pathname);
  const routeKey = matched ? `${matched.route.path}:${pathname}` : '';

  useEffect(() => {
    let active = true;
    setLoaded(null);
    setLoadError('');
    if (!matched) return () => { active = false; };
    matched.route.load().then(value => { if (active) setLoaded({ key: routeKey, Page: value.default }); })
      .catch(error => { if (active) setLoadError(error.message || '页面加载失败'); });
    return () => { active = false; };
  }, [routeKey, locationKey]);

  const Page = loaded?.key === routeKey ? loaded.Page : null;
  return html`<div class="dhcp-standalone">
    <header class="page-toolbar"><h1>${manifest?.title || 'DHCP'}</h1></header>
    ${manifest?.navigation?.length ? html`<nav class="form-actions">${manifest.navigation.map(item => html`<a class="button" href=${href(item.path)}>${item.title}</a>`)}</nav>` : null}
    ${manifestError ? html`<div class="notice-card error">${manifestError}</div>` : null}
    ${!matched && manifest ? html`<div class="notice-card">页面不存在</div>` : null}
    ${loadError ? html`<div class="notice-card error">${loadError}</div>` : null}
    ${matched && !Page && !loadError ? html`<div class="loading-state">加载中…</div>` : null}
    ${Page ? html`<${Page} api=${api} params=${matched.params} search=${current.search} navigate=${navigate} href=${href} ...${matched.route.props || {}} />` : null}
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
