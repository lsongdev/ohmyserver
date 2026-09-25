import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content;
if (!apiBase) throw new Error('missing module api-base');
const moduleRoot = new URL(`${apiBase.replace(/\/+$/, '')}/`, location.origin);
const moduleID = moduleRoot.pathname.split('/').filter(Boolean).at(-1);

function scopedURL(path) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) {
    throw new TypeError('module API path must start with a single /');
  }
  const url = new URL(path.slice(1), moduleRoot);
  if (url.origin !== moduleRoot.origin || !url.pathname.startsWith(moduleRoot.pathname)) {
    throw new TypeError('module API path escaped its mount');
  }
  return url;
}

function assetURL(path) {
  if (typeof path !== 'string' || path.startsWith('/') || /(^|\/)\.\.?($|\/)|[?#\\]/.test(path) || /^[a-z][a-z0-9+.-]*:/i.test(path)) {
    throw new TypeError('invalid module asset path');
  }
  const url = new URL(path, moduleRoot);
  if (url.origin !== moduleRoot.origin || !url.pathname.startsWith(`${moduleRoot.pathname}assets/`)) {
    throw new TypeError('module asset must stay inside its assets directory');
  }
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
  const response = await fetch(scopedURL(path), { ...options, body, headers, credentials: 'same-origin' });
  if (response.status === 204) return null;
  const type = response.headers.get('content-type') || '';
  const payload = type.includes('application/json') ? await response.json() : null;
  if (!response.ok) {
    const error = payload?.error || {};
    const failure = new Error(error.message || response.statusText || `HTTP ${response.status}`);
    failure.status = response.status;
    failure.code = error.code;
    throw failure;
  }
  return payload?.data ?? payload;
}

const transport = {
  request,
  fetchImpl: (path, options = {}) => fetch(scopedURL(path), { credentials: 'same-origin', ...options }),
  apiURL: path => scopedURL(path).href,
  websocketURL: path => {
    const url = scopedURL(path);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url.href;
  },
};

function href(path) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) {
    throw new TypeError('module navigation path must start with a single /');
  }
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

function findRoute(routes, pathname) {
  let best = null;
  let score = -1;
  for (const route of routes) {
    const params = matchRoute(route.path, pathname);
    if (params === null) continue;
    const next = route.path.split('/').filter(Boolean).filter(segment => !segment.startsWith(':')).length;
    if (next > score) {
      best = { route, params };
      score = next;
    }
  }
  return best;
}

function loadStyles(styles = []) {
  for (const path of styles) {
    const url = assetURL(path);
    if ([...document.querySelectorAll('link[data-module-style]')].some(link => link.href === url.href)) continue;
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = url.href;
    link.dataset.moduleStyle = moduleID;
    document.head.append(link);
  }
}

async function loadDefinition() {
  const response = await fetch(new URL('manifest.json', moduleRoot), {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) throw new Error(`module manifest failed (${response.status})`);
  const manifest = await response.json();
  if (manifest.contractVersion !== 1 || manifest.id !== moduleID || typeof manifest.entry !== 'string') {
    throw new Error('invalid module manifest');
  }
  loadStyles(manifest.styles);
  document.title = manifest.title || manifest.id;
  const runtime = (await import(assetURL(manifest.entry).href)).default;
  if (!runtime || runtime.id !== manifest.id || !Array.isArray(runtime.routes) || typeof runtime.createAPI !== 'function') {
    throw new Error('invalid module runtime');
  }
  for (const key of ['title', 'icon', 'group', 'entry', 'home', 'navigation', 'styles']) {
    if (Object.hasOwn(runtime, key)) throw new Error(`module metadata ${key} belongs in manifest.json`);
  }
  return { manifest, runtime, api: runtime.createAPI(transport) };
}

function App() {
  const [definition, setDefinition] = useState(null);
  const [definitionError, setDefinitionError] = useState('');
  const [locationKey, setLocationKey] = useState(location.hash);
  const [loaded, setLoaded] = useState(null);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    let active = true;
    loadDefinition()
      .then(value => { if (active) setDefinition(value); })
      .catch(error => { if (active) setDefinitionError(error.message || 'module failed to load'); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    const update = () => setLocationKey(location.hash);
    addEventListener('hashchange', update);
    return () => removeEventListener('hashchange', update);
  }, []);

  const current = readLocation();
  const pathname = current.pathname === '/' ? (definition?.manifest.home || '/') : current.pathname;
  const matched = definition ? findRoute(definition.runtime.routes, pathname) : null;
  const routeKey = matched ? `${matched.route.path}:${pathname}` : '';

  useEffect(() => {
    let active = true;
    setLoaded(null);
    setLoadError('');
    if (!matched) return () => { active = false; };
    matched.route.load()
      .then(value => { if (active) setLoaded({ key: routeKey, Page: value.default }); })
      .catch(error => { if (active) setLoadError(error.message || 'page failed to load'); });
    return () => { active = false; };
  }, [routeKey, locationKey]);

  const Page = loaded?.key === routeKey ? loaded.Page : null;
  const manifest = definition?.manifest;
  return html`<main class="module-standalone">
    <header class="page-toolbar"><h1>${manifest?.title || moduleID}</h1></header>
    ${manifest?.navigation?.length ? html`<nav class="button-group">${manifest.navigation.map(item => html`<a class="button" href=${href(item.path)}>${item.title}</a>`)}</nav>` : null}
    ${definitionError ? html`<div class="notice-card error" role="alert">${definitionError}</div>` : null}
    ${definition && !matched ? html`<div class="notice-card">页面不存在</div>` : null}
    ${loadError ? html`<div class="notice-card error" role="alert">${loadError}</div>` : null}
    ${definition && matched && !Page && !loadError ? html`<div class="loading-state">加载中…</div>` : null}
    ${Page ? html`<${Page} api=${definition.api} params=${matched.params} search=${current.search} navigate=${navigate} href=${href} ...${matched.route.props || {}} />` : null}
  </main>`;
}

render(html`<${App} />`, document.getElementById('app'));
